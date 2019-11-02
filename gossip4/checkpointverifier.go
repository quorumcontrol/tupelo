package gossip4

import (
	"bytes"
	"context"
	"fmt"
	"math/big"
	"sync"
	"time"

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
	verifyAct     *actor.PID
	rebroadcaster *actor.PID
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

	cv := &checkpointVerifier{
		node:          node,
		nodeActor:     nodeActor,
		group:         group,
		queued:        make(map[uint64]*Checkpoint),
		txValidator:   validator,
		verKeys:       verKeys,
		lock:          new(sync.RWMutex),
		logger:        node.logger,
		recentCommits: recentCache,
	}

	act := actor.EmptyRootContext.Spawn(actor.PropsFromFunc(cv.Receive))
	cv.verifyAct = act
	return cv, nil
}

func (cv *checkpointVerifier) validate(ctx context.Context, pID peer.ID, msg *pubsub.Message) bool {
	// TODO: should we we do more checks here?
	// like: verify that it came from a signer? (maybe we don't care)
	// should we just return true if it's our own CP?
	cv.logger.Debugf("checkpoint verifier running")

	cp := &Checkpoint{}
	err := cbornode.DecodeInto(msg.Data, cp)
	if err != nil {
		cv.logger.Errorf("error decoding: %v", err)
		return false
	}

	if cv.node.p2pNode.Identity() == pID.Pretty() {
		cv.logger.Debugf("received own message %d %v", cp.Height, cp.Signature.Signers)
		if cv.inFlight == nil {
			cv.inFlight = cp
		}
		// if we've already signed it we can just return true
		if cp.Signature.Signers[cv.node.signerIndex] > 0 {
			return true
		}
	}

	return cv.validateCheckpoint(ctx, cp)
}

func (cv *checkpointVerifier) validateCheckpoint(ctx context.Context, cp *Checkpoint) bool {
	cv.logger.Debugf("running checkpoint for: (%d): %s", cp.Height, cp.CurrentState.String())

	// If the height of the checkpoint is lower than the checkpoint we are already at
	// then we can just ignore this checkpoint
	height := cv.node.latestCheckpoint.Height
	if height > 0 && cp.Height < height {
		cv.logger.Debugf("received lower height checkpoint, ignoring %d", cp.Height)
		return false
	}

	cv.logger.Debugf("verifying signatures %d %v", cp.Height, cp.Signature.Signers)
	sigVerified, err := verifySignature(ctx, cp.CurrentState.Bytes(), &cp.Signature, cv.verKeys)
	if !sigVerified || err != nil {
		cv.logger.Warningf("unverified signature %d (err: %v)", cp.Height, err)
		return false
	}

	if cv.recentCommits.Contains(cp.Height) {
		cv.logger.Debugf("already committed %d, stopping", cp.Height)
		return false
	}

	fut := cv.verifyAct.RequestFuture(&cpMessage{ctx: ctx, cp: cp}, 5*time.Second)
	resp, err := fut.Result()
	if err != nil {
		cv.logger.Warningf("error getting request: %v", err)
		return false
	}
	return resp.(bool)
}

type cpMessage struct {
	cp  *Checkpoint
	ctx context.Context
}

func (cv *checkpointVerifier) Receive(actorContext actor.Context) {
	switch msg := actorContext.Message().(type) {
	case *actor.Started:
		cv.rebroadcaster = actorContext.Spawn(actor.PropsFromProducer(func() actor.Actor {
			return &rebroadcaster{
				logger: cv.logger,
				pubsub: cv.node.pubsub,
			}
		}))
	case *cpMessage:
		valid := cv.actorValidation(msg.ctx, msg.cp, actorContext)
		actorContext.Respond(valid)
	}
}

func (cv *checkpointVerifier) actorValidation(ctx context.Context, cp *Checkpoint, actorContext actor.Context) bool {
	cv.logger.Debugf("actorValidator running: %d %v", cp.Height, cp.Signature.Signers)
	if cv.recentCommits.Contains(cp.Height) {
		cv.logger.Debugf("already committed %d", cp.Height)
		return false
	}

	height := cv.node.latestCheckpoint.Height

	// if this checkpoint isn't the next in the sequence then just queue it up
	// TODO: do we really want to propogate this to the network here since we havne't
	// evaluated its truthyness? maybe it should be like Txs which we verify are
	// internally consistent at least, but have to wait to see if they are valid
	// as a state transition
	if height > 0 && cp.Height != height+1 {
		cv.logger.Debugf("queueing up checkpoint %d", cp.Height)
		cv.queued[cp.Height] = cp
		return true
	}

	// if we have reached consensus already, then pass it to the node actor
	// and still let pubsub forward the message out to the other subscribers
	if uint64(cp.SignerCount()) >= cv.group.QuorumCount() {
		// then commit!
		cv.commit(ctx, cp)
		return true
	}
	if cv.inFlight != nil && !cv.inFlight.Equals(cp) {
		return false // TODO: for now we will ignore... really we'll need deadlock detection here
	}

	if cv.inFlight != nil && cv.inFlight.Equals(cp) {
		cv.logger.Debugf("already have this checkpoint (%d %s) in flight, checking signatures", cp.Height, cv.inFlight.CurrentState.String())
		// do signature stuff and return
		oursCount, theirsCount := signerDiff(cv.inFlight.Signature, cp.Signature)
		if oursCount == 0 && theirsCount == 0 {
			cv.logger.Debugf("no new signatures %d: %v", cp.Height, cv.inFlight.Signature.Signers)
			return true // TODO: really? do we want to to this?
		}
		if theirsCount > 0 {
			cv.logger.Debugf("theirs has a new signature, combining and rebroadcasting %d (ours: %v) (theirs: %v)", cp.Height, cv.inFlight.Signature.Signers, cp.Signature.Signers)
			// that means if we combine these sigs, we'll end up with a better sig
			// so do that and queue up the combined sig
			newSig, err := sigfuncs.AggregateBLSSignatures([]*signatures.Signature{&cv.inFlight.Signature, &cp.Signature})
			if err != nil {
				cv.logger.Errorf("error aggregating sigs: %v", err)
				return false
			}
			cv.inFlight.Signature = *newSig

			actorContext.Send(cv.rebroadcaster, cv.inFlight)

			// if we have reached consensus , then pass it to the node actor
			if uint64(cp.SignerCount()) >= cv.group.QuorumCount() {
				// then commit!
				cv.commit(ctx, cp)
			}

			return false
		}
		// otherwise we have a strictly better signature, so we can just rebroadcast ours (maybe?)
		cv.logger.Debugf("ours is a strictly better signature %d: %v", cp.Height, cv.inFlight.Signature.Signers)

		actorContext.Send(cv.rebroadcaster, cv.inFlight)

		return false // ignoring this strictly worse signature for now
	}

	cv.logger.Debugf("validating checkpoint %d", cp.Height)
	valid := cv.verify(ctx, cp)
	if valid {
		cv.logger.Debugf("checkpoint valid, setting inflight to true")
		cv.inFlight = cp
	}

	return true
}

func (cv *checkpointVerifier) commit(ctx context.Context, cp *Checkpoint) {
	cv.logger.Debugf("committing checkpoint %d", cp.Height)
	cv.recentCommits.Add(cp.Height, struct{}{})
	actor.EmptyRootContext.Send(cv.nodeActor, cp)

	if cv.inFlight != nil && cv.inFlight.Equals(cp) {
		cv.inFlight = nil
	}

	next, ok := cv.queued[cp.Height+1]
	if ok {
		cv.logger.Debugf("found queued cp, running")
		go cv.validateCheckpoint(ctx, next)
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

func (cv *checkpointVerifier) verify(ctx context.Context, cp *Checkpoint) bool {

	// check the difficulty and if it's not enough
	// we can just drop the checkpoint
	num := big.NewInt(0)
	num.SetBytes(cp.CurrentState.Bytes())
	if !(num.TrailingZeroBits() >= difficultyThreshold) {
		cv.logger.Warningf("received message with not enough difficulty, ignoring")
		return false
	}

	// Does the previous match?
	if !cp.Previous.Equals(cv.node.latestCheckpoint.CurrentState) {
		cv.logger.Warningf("checkpoint previous isn't consistent %d", cp.Height)
		return false
	}

	// verify all the transactions are ok and build off previous
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
