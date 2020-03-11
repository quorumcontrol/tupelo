package benchmark

import (
	"context"
	"fmt"
	"sync"

	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/messages/v2/build/go/gossip"
	"github.com/quorumcontrol/messages/v2/build/go/signatures"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/client/pubsubinterfaces"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/types"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
	sigfuncs "github.com/quorumcontrol/tupelo-go-sdk/signatures"
)

var detectorLogger = logging.Logger("faultdetector")

type roundConflictSet map[cid.Cid]*gossip.RoundConfirmation

func isQuorum(group *types.NotaryGroup, sig *signatures.Signature) bool {
	return uint64(sigfuncs.SignerCount(sig)) >= group.QuorumCount()
}

func (rcs roundConflictSet) add(group *types.NotaryGroup, confirmation *gossip.RoundConfirmation) (makesQuorum bool, updated *gossip.RoundConfirmation, err error) {
	ctx := context.TODO()

	cid, err := cid.Cast(confirmation.RoundCid)
	if err != nil {
		return false, nil, fmt.Errorf("error casting round cid: %w", err)
	}

	existing, ok := rcs[cid]
	if !ok {
		// this is the first time we're seeing the completed round,
		// just add it to the conflict set and move on
		rcs[cid] = confirmation
		return isQuorum(group, confirmation.Signature), confirmation, nil
	}

	// otherwise we've already seen a confirmation for this, let's combine the signatures
	newSig, err := sigfuncs.AggregateBLSSignatures(ctx, []*signatures.Signature{existing.Signature, confirmation.Signature})
	if err != nil {
		return false, nil, err
	}

	existing.Signature = newSig
	rcs[cid] = existing

	return isQuorum(group, existing.Signature), existing, nil
}

type conflictSetHolder map[uint64]roundConflictSet

type FaultDetector struct {
	sync.RWMutex

	P2PNode  p2p.Node
	Group    *types.NotaryGroup
	DagStore nodestore.DagStore

	rounds conflictSetHolder
	logger logging.EventLogger

	subscription *pubsub.Subscription
}

func (fd *FaultDetector) Start(ctx context.Context) error {
	if fd.logger == nil {
		fd.logger = detectorLogger
	}

	if fd.rounds == nil {
		fd.rounds = make(conflictSetHolder)
	}

	sub, err := fd.P2PNode.GetPubSub().Subscribe(fd.Group.ID)
	if err != nil {
		return fmt.Errorf("error subscribing %w", err)
	}
	fd.subscription = sub

	go func() {
		for {
			msg, err := sub.Next(ctx)
			if err != nil {
				fd.logger.Warningf("error getting sub message: %v", err)
				return
			}
			if err := fd.handleMessage(ctx, msg); err != nil {
				fd.logger.Warningf("error handling pubsub message: %v", err)
				return
			}
		}
	}()
	return nil
}

func (fd *FaultDetector) Stop() {
	if fd.subscription != nil {
		fd.subscription.Cancel()
	}
}

func (fd *FaultDetector) handleMessage(ctx context.Context, msg *pubsub.Message) error {
	fd.logger.Debugf("handling message")

	confirmation, err := pubsubMessageToRoundConfirmation(ctx, msg)
	if err != nil {
		return fmt.Errorf("error handling: %w", err)
	}

	err = sigfuncs.RestoreBLSPublicKey(ctx, confirmation.Signature, fd.verKeys())
	if err != nil {
		return fmt.Errorf("error restoring BLS key: %w", err)
	}

	verified, err := sigfuncs.Valid(ctx, confirmation.Signature, confirmation.RoundCid, nil)
	if !verified || err != nil {
		return fmt.Errorf("signature invalid with error: %w", err)
	}

	fd.Lock()
	defer fd.Unlock()

	roundCid, err := cid.Cast(confirmation.RoundCid)
	if err != nil {
		return fmt.Errorf("error casting round cid: %w", err)
	}

	fd.logger.Infof("round %d (%s) from %s", confirmation.Height, roundCid.String(), msg.GetFrom().Pretty())

	conflictSet, ok := fd.rounds[confirmation.Height]
	if !ok {
		conflictSet = make(roundConflictSet)
	}

	madeQuorum, _, err := conflictSet.add(fd.Group, confirmation)
	if err != nil {
		return fmt.Errorf("error adding to conflictset: %w", err)
	}
	fd.rounds[confirmation.Height] = conflictSet
	if madeQuorum {

		wrappedConfirmation := types.WrapRoundConfirmation(confirmation)
		wrappedConfirmation.SetStore(fd.DagStore)

		// fetch the completed round and confirmation here as no ops so that they are cached
		wrappedCompletedRound, err := wrappedConfirmation.FetchCompletedRound(ctx)
		if err != nil {
			return err
		}

		wrappedCheckpoint, err := wrappedCompletedRound.FetchCheckpoint(ctx)
		if err != nil {
			return err
		}

		fd.logger.Infof("round %d decided with len(%d)", confirmation.Height, len(wrappedCheckpoint.AddBlockRequests()))
	}

	if len(conflictSet) > 1 {
		keys := make([]string, len(conflictSet))
		i := 0
		for roundCid := range conflictSet {
			keys[i] = roundCid.String()
			i++
		}

		fd.logger.Warningf("conflicting round signatures %d (%v), this one %s from: %s", confirmation.Height, keys, roundCid.String(), msg.GetFrom().Pretty())

		for roundCid, confirmation := range conflictSet {
			wrappedConfirmation := types.WrapRoundConfirmation(confirmation)
			wrappedConfirmation.SetStore(fd.DagStore)

			// fetch the completed round and confirmation here as no ops so that they are cached
			wrappedCompletedRound, err := wrappedConfirmation.FetchCompletedRound(ctx)
			if err != nil {
				return err
			}

			stateCid, err := cid.Cast(wrappedCompletedRound.Value().StateCid)
			if err != nil {
				return fmt.Errorf("error casting state cid: %v", err)
			}

			wrappedCheckpoint, err := wrappedCompletedRound.FetchCheckpoint(ctx)
			if err != nil {
				return err
			}

			abrs := wrappedCheckpoint.AddBlockRequests()
			txIds := make([]string, len(abrs))
			for i, abrBytes := range abrs {
				id, err := cid.Cast(abrBytes)
				if err != nil {
					return fmt.Errorf("error casting abr bytes: %w", err)
				}
				txIds[i] = id.String()
			}

			fd.logger.Warningf("round %d id: %s, transactions from this commit: (len: %d) %v --- checkpointCid: %s, stateCID: %s", confirmation.Height, roundCid.String(), len(txIds), txIds, wrappedCheckpoint.CID(), stateCid.String())
		}
	}

	return nil
}

// TODO: we can cache this
func (fd *FaultDetector) verKeys() []*bls.VerKey {
	keys := make([]*bls.VerKey, len(fd.Group.AllSigners()))
	for i, s := range fd.Group.AllSigners() {
		keys[i] = s.VerKey
	}
	return keys
}

func pubsubMessageToRoundConfirmation(ctx context.Context, msg pubsubinterfaces.Message) (*gossip.RoundConfirmation, error) {
	bits := msg.GetData()
	confirmation := &gossip.RoundConfirmation{}
	err := cbornode.DecodeInto(bits, confirmation)
	return confirmation, err
}
