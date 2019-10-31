package gossip4

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	sigfuncs "github.com/quorumcontrol/tupelo-go-sdk/signatures"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	cbornode "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/build/go/services"
	"github.com/quorumcontrol/messages/build/go/signatures"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
)

// if there are 2/3 of signers, then commit the checkpoint

// otherwise, see if we've already valiated and if we have then see if we can add to the sigs
// if we can then do and republish

// on commit
// set the n.latestCheckpoint to this checkpoint
// get rid of all inflight that was in the new transactions of the commit
// take all of the inflight transactions and play them on top of this doing the normal
// stuff of finding 0s

type checkpointVerifier struct {
	nodeActor     *actor.PID
	node          *Node
	group         *types.NotaryGroup
	queued        map[uint64]*Checkpoint
	inFlight      *Checkpoint
	txValidator   *transactionValidator
	verKeys       []*bls.VerKey
	lock          *sync.RWMutex
	logger        logging.EventLogger
	recentCommits *lru.Cache
}

func newCheckpointVerifier(node *Node, nodeActor *actor.PID, group *types.NotaryGroup) (*checkpointVerifier, error) {
	validator, err := newTransactionValidator(node.logger, group, nil) // passing in nil for node actor because we don't ever want this wone talking over there
	if err != nil {
		return nil, fmt.Errorf("error creating validator: %v", err)
	}

	recentCache, err := lru.New(200)
	if err != nil {
		return nil, fmt.Errorf("error creating cache: %v", err)
	}

	verKeys := make([]*bls.VerKey, group.Size())
	for i, s := range group.AllSigners() {
		verKeys[i] = s.VerKey
	}

	return &checkpointVerifier{
		node:          node,
		nodeActor:     nodeActor,
		group:         group,
		queued:        make(map[uint64]*Checkpoint),
		txValidator:   validator,
		verKeys:       verKeys,
		lock:          new(sync.RWMutex),
		logger:        node.logger,
		recentCommits: recentCache,
	}, nil
}

func (cv *checkpointVerifier) validate(ctx context.Context, pID peer.ID, msg *pubsub.Message) bool {
	// TODO: should we we do more checks here?
	// like: verify that it came from a signer? (maybe we don't care)
	cv.logger.Debugf("checkpoint verifier running")

	cp := &Checkpoint{}
	err := cbornode.DecodeInto(msg.Data, cp)
	if err != nil {
		cv.logger.Errorf("error decoding: %v", err)
		return false
	}

	if cv.recentCommits.Contains(cp.CurrentState.String()) {
		cv.logger.Debugf("already committed, skipping")
		return true
	}

	// If the height of the checkpoint is lower than the checkpoint we are already at
	// then we can just ignore this checkpoint
	cv.node.lock.RLock()
	height := cv.node.latestCheckpoint.Height
	cv.node.lock.RUnlock()
	if height > 0 && cp.Height <= height {
		cv.logger.Debugf("received lower height checkpoint, ignoring")
		return false
	}

	cv.logger.Debugf("verifying signatures")
	sigVerified, err := verifySignature(ctx, cp.CurrentState.Bytes(), &cp.Signature, cv.verKeys)
	if !sigVerified || err != nil {
		cv.logger.Warningf("unverified signature (err: %v)", err)
		return false
	}

	var signerCount int
	for _, cnt := range cp.Signature.Signers {
		if cnt > 0 {
			signerCount++
		}
	}

	// if we have reached consensus already, then pass it to the node actor
	// and still let pubsub forward the message out to the other subscribers
	if uint64(signerCount) >= cv.group.QuorumCount() {
		// then commit!
		cv.commit(ctx, cp)
		return true
	}

	// if this checkpoint isn't the next in the sequence then just queue it up
	// TODO: do we really want to propogate this to the network here since we havne't
	// evaluated its truthyness? maybe it should be like Txs which we verify are
	// internally consistent at least, but have to wait to see if they are valid
	// as a state transition
	if height > 0 && cp.Height != height+1 {
		cv.logger.Debugf("queueing up checkpoint")
		cv.queued[cp.Height] = cp
		return true
	}

	cv.lock.Lock()
	defer cv.lock.Unlock()

	if cv.inFlight != nil && cv.inFlight.Equals(cp) {
		cv.logger.Debugf("already have a checkpoint in flight, checking signatures")
		// do signature stuff and return
		oursCount, theirsCount := signerDiff(cv.inFlight.Signature, cp.Signature)
		if oursCount == 0 && theirsCount == 0 {
			cv.logger.Debugf("no new signatures: %v", cv.inFlight.Signature.Signers)
			return true // TODO: really? do we want to to this?
		}
		if theirsCount > 0 {
			cv.logger.Debugf("theirs has a new signature, combining and rebroadcasting (ours: %v) (theirs: %v)", cv.inFlight.Signature.Signers, cp.Signature.Signers)
			// that means if we combine these sigs, we'll end up with a better sig
			// so do that and queue up the combined sig
			newSig, err := sigfuncs.AggregateBLSSignatures([]*signatures.Signature{&cv.inFlight.Signature, &cp.Signature})
			if err != nil {
				cv.logger.Errorf("error aggregating sigs: %v", err)
				return false
			}
			cv.inFlight.Signature = *newSig

			var signerCount int
			for _, cnt := range newSig.Signers {
				if cnt > 0 {
					signerCount++
				}
			}

			// if we have reached consensus , then pass it to the node actor
			if uint64(signerCount) >= cv.group.QuorumCount() {
				// then commit!
				cv.commit(ctx, cp)
			}

			go func() {
				sw := &safewrap.SafeWrap{}
				cv.node.pubsub.Publish(commitTopic, sw.WrapObject(cv.inFlight).RawData())
			}()
			return false
		}
		// otherwise we have a strictly better signature, so we can just rebroadcast ours
		cv.logger.Debugf("ours is a strictly better signature: %v", cv.inFlight.Signature.Signers)
	}

	cv.logger.Debugf("validating checkpoint")
	valid := cv.validateCheckpoint(ctx, cp)
	if valid {
		cv.logger.Debugf("checkpoint valid, setting inflight to true")
		cv.inFlight = cp
	}

	return true
}

func (cv *checkpointVerifier) commit(_ context.Context, cp *Checkpoint) {
	cv.logger.Debugf("committing checkpoint")
	cv.recentCommits.Add(cp.CurrentState.String(), struct{}{})
	actor.EmptyRootContext.Send(cv.nodeActor, cp)
	if cv.inFlight.Equals(cp) {
		cv.inFlight = nil
	}
}

func signerDiff(ourSig signatures.Signature, theirSig signatures.Signature) (ours int, theirs int) {
	for i, cnt := range ourSig.Signers {
		if cnt > 0 && theirSig.Signers[i] == 0 {
			ours++
			continue
		}
		if cnt == 0 && theirSig.Signers[i] > 0 {
			theirs++
			continue
		}
	}
	return
}

func (cv *checkpointVerifier) validateCheckpoint(ctx context.Context, cp *Checkpoint) bool {

	// check the difficulty and if it's not enough
	// we can just drop the checkpoint
	num := big.NewInt(0)
	num.SetBytes(cp.CurrentState.Bytes())
	if !(num.TrailingZeroBits() >= difficultyThreshold) {
		cv.logger.Warningf("received message with not enough difficulty, ignoring")
		return false
	}

	// othwerwise lets verify the transactions
	checkpointVerified, err := cv.validateTransactions(ctx, cp)
	if !checkpointVerified || err != nil {
		cv.logger.Warningf("unverified checkpoint (err: %v)", err)
		return false
	}

	return true
}

func (cv *checkpointVerifier) validateTransactions(ctx context.Context, cp *Checkpoint) (bool, error) {
	// does its previosuTip match the current previous
	// do the new transactions create the same tip
	// are all the new Txs valid

	// Does the previous match?
	if !cp.Previous.Equals(cv.node.latestCheckpoint.CurrentState) {
		cv.logger.Warningf("checkpoint previous isn't consistent")
		return false, nil
	}

	txs, err := cv.cidsToAbrs(ctx, cp.Transactions)
	if err != nil {
		return false, fmt.Errorf("error getting txs: %v", err)
	}

	// for each of these transactions, see if they are in the inflight (which means they are validated)
	// or go and validate the transaction
	for i, abr := range txs {
		_, ok := cv.node.inFlight[cp.Transactions[i].String()]
		if ok {
			// it's already been validated
			continue
		}
		valid := cv.txValidator.validateAbr(ctx, abr)
		if !valid {
			cv.logger.Warningf("received invalid Tx", err)
			return false, nil
		}
	}

	tempHamt := cv.node.latestCheckpoint.node.Copy()
	// now for each of these Txs, make sure they build off the previous tip
	// if they do, add them to a new temporary hamt, and make sure their addition
	// adds up to the correct currentState
	for i, abr := range txs {
		var id cid.Cid
		err := cv.node.latestCheckpoint.node.Find(ctx, string(abr.ObjectId), &id)
		if err != nil && err != hamt.ErrNotFound {
			return false, fmt.Errorf("error getting from hamt: %v", err)
		}

		if !id.Equals(cid.Undef) {
			existing, err := cv.getAddBlockRequest(ctx, id)
			if err != nil {
				return false, fmt.Errorf("error getting exsting: %v", err)
			}
			if !bytes.Equal(existing.NewTip, abr.PreviousTip) {
				cv.logger.Warningf("received invalid Tx, built on wrong tip %v", err)
				return false, nil
			}
		}

		err = tempHamt.Set(ctx, string(abr.ObjectId), cp.Transactions[i])
		if err != nil {
			return false, fmt.Errorf("error setting hamt: %v", err)
		}
	}

	memCborIpldStore := dagStoreToCborIpld(nodestore.MustMemoryStore(ctx))
	id, err := memCborIpldStore.Put(ctx, tempHamt)
	if err != nil {
		return false, fmt.Errorf("error serializing node: %v", err)
	}

	if !id.Equals(cp.CurrentState) {
		cv.logger.Warningf("checkpoint has a currentState which is not the result of playing the included transactions")
		return false, nil
	}

	return true, nil
}

func (cv *checkpointVerifier) getAddBlockRequest(ctx context.Context, id cid.Cid) (*services.AddBlockRequest, error) {
	// first check to see if it's in the node inflight which saves lookup and conversion
	abr, ok := cv.node.inFlight[id.String()]
	if ok {
		return abr, nil
	}

	// if not, lets get it either from local store, or go out to bitswap to get it
	err := cv.node.hamtStore.Get(ctx, id, abr)
	if err != nil {
		return nil, fmt.Errorf("error getting abr: %v", err)
	}
	return abr, nil
}

func (cv *checkpointVerifier) cidsToAbrs(ctx context.Context, cids []cid.Cid) ([]*services.AddBlockRequest, error) {
	txs := make([]*services.AddBlockRequest, len(cids))
	for i, id := range cids {
		abr, err := cv.getAddBlockRequest(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("error getting tx: %v", err)
		}
		txs[i] = abr
	}
	return txs, nil
}
