package gossip4

import (
	"context"
	"fmt"
	"math/big"

	"github.com/AsynkronIT/protoactor-go/actor"
	lru "github.com/hashicorp/golang-lru"
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/messages/build/go/services"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
)

// on receiving a new checkpoint
// verify that it is internally consistent
// that all the transactions are *internally* valid
// that it has enough difficulty

// we want to allow the continuation of the propogation if:
//   this is strictly better then the signature we know about (includes all of ours too)
//   or this has 2/3

// otherwise stop propgation because we are about to broadcast a better (combined on)

// stop propogation, but still send the checkpoint to the node actor which will
// consolidate, sign if necessary and check that it is actually consistent (previous matches, etc)

type checkpointVerifier struct {
	nodeActor *actor.PID
	node      *Node
	group     *types.NotaryGroup
	// queued        map[uint64]*Checkpoint
	// inFlight      *Checkpoint
	txValidator *transactionValidator
	verKeys     []*bls.VerKey
	// lock          *sync.RWMutex
	logger   logging.EventLogger
	inflight *lru.Cache
	// recentCommits *lru.Cache
	// verifyAct     *actor.PID
	// rebroadcaster *actor.PID
	// consolidator *actor.PID
}

func newCheckpointVerifier(node *Node, nodeActor *actor.PID, group *types.NotaryGroup) (*checkpointVerifier, error) {
	validator, err := newTransactionValidator(node.logger, group, nil) // passing in nil for node actor because we don't ever want this wone talking over there
	if err != nil {
		return nil, fmt.Errorf("error creating validator: %v", err)
	}

	inflightCache, err := lru.New(200)
	if err != nil {
		return nil, fmt.Errorf("error creating cache: %v", err)
	}

	verKeys := make([]*bls.VerKey, group.Size())
	for i, s := range group.AllSigners() {
		verKeys[i] = s.VerKey
	}

	cv := &checkpointVerifier{
		node:        node,
		nodeActor:   nodeActor,
		group:       group,
		txValidator: validator,
		verKeys:     verKeys,
		logger:      node.logger,
		inflight:    inflightCache,
		// consolidator: actor.EmptyRootContext.Spawn(newCheckpointConsolidatorProps(node.logger)),
	}

	// act := actor.EmptyRootContext.Spawn(actor.PropsFromFunc(cv.Receive))
	// cv.verifyAct = act
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

	// if this message is ours, we can trust ourselves
	// TODO: make sure signing is strictly enforced
	// no need to tell ourselves about a node or check
	// if it should be sent.
	if cv.node.p2pNode.Identity() == pID.Pretty() {
		return true
	}

	validated := cv.validateCheckpoint(ctx, cp)
	if !validated {
		return false
	}

	// even though we are or aren't going to propogate this message
	// we should let the node know about it because it's internally consistent
	// and the node is what combines signatures, verifies it more, etc
	actor.EmptyRootContext.Send(cv.nodeActor, cp)

	return cv.shouldPropogate(ctx, cp)
}

func (cv *checkpointVerifier) shouldPropogate(ctx context.Context, cp *Checkpoint) bool {
	didAdd, _ := cv.inflight.ContainsOrAdd(cp.Height, cp)
	if didAdd {
		return true // this was a totally fresh checkpoint
	}
	// otherwise lets grab the existing one
	existingInter, ok := cv.inflight.Get(cp.Height)
	if !ok {
		cv.logger.Warningf("unknown case here where nothing in the cache")
		return false // seems like something weird in this case TODO: think through WHY?
	}
	existing := existingInter.(*Checkpoint)
	ours, theirs := signerDiff(existing.Signature, cp.Signature)
	if ours == 0 && theirs > 0 {
		// theirs is strictly better, allowing propogation
		cv.inflight.Add(cp.Height, cp)
		return true
	}
	// otherwise we know we'll get a better signature
	return false
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

	// if no signers have signed this thing, we can just ignore it
	if cp.SignerCount() == 0 {
		return false
	}

	// check the difficulty and if it's not enough
	// we can just drop the checkpoint
	num := big.NewInt(0)
	num.SetBytes(cp.CurrentState.Bytes())
	if !(num.TrailingZeroBits() >= difficultyThreshold) {
		cv.logger.Warningf("received message with not enough difficulty, ignoring")
		return false
	}

	// make sure the signatures verify
	cv.logger.Debugf("verifying signatures %d %v", cp.Height, cp.Signature.Signers)
	sigVerified, err := verifySignature(ctx, cp.CurrentState.Bytes(), &cp.Signature, cv.verKeys)
	if !sigVerified || err != nil {
		cv.logger.Warningf("unverified signature %d (err: %v)", cp.Height, err)
		return false
	}

	// if 2/3 of the signers have signed this, we don't need to validate
	// TODO: are we sure we want to do this? Maybe we should still validate for slashing
	// offenses
	if uint64(cp.SignerCount()) >= cv.group.QuorumCount() {
		return true
	}

	// verify all the transactions are ok and build off previous
	checkpointVerified, err := cv.validateTransactions(ctx, cp)
	if !checkpointVerified || err != nil {
		cv.logger.Warningf("unverified checkpoint (err: %v)", err)
		return false
	}

	return false
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
		_, ok := cv.node.inFlightAbrs[cp.Transactions[i].String()]
		if ok {
			// it's already been validated
			continue
		}
		valid := cv.txValidator.validateAbr(ctx, abr)
		if !valid {
			cv.logger.Warningf("received invalid Tx", err)
			return false, nil
		}
		// if it is valid, it probably means the node doesn't know about it so let it know
		actor.EmptyRootContext.Send(cv.nodeActor, abr)
	}

	// Then make sure the checkpoint is internally consistent... that updating the previous to the current using
	// valid transactions equals the updated current state

	tempHamt := cv.node.latestCheckpoint.node.Copy()

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
	abr, ok := cv.node.inFlightAbrs[id.String()]
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

// // now for each of these Txs, make sure they build off the previous tip
// // if they do, add them to a new temporary hamt, and make sure their addition
// // adds up to the correct currentState
// for i, abr := range txs {
// 	var id cid.Cid
// 	err := cv.node.latestCheckpoint.node.Find(ctx, string(abr.ObjectId), &id)
// 	if err != nil && err != hamt.ErrNotFound {
// 		return false, fmt.Errorf("error getting from hamt: %v", err)
// 	}

// 	if !id.Equals(cid.Undef) {
// 		existing, err := cv.getAddBlockRequest(ctx, id)
// 		if err != nil {
// 			return false, fmt.Errorf("error getting exsting: %v", err)
// 		}
// 		if !bytes.Equal(existing.NewTip, abr.PreviousTip) {
// 			cv.logger.Warningf("received invalid Tx, built on wrong tip %v", err)
// 			return false, nil
// 		}
// 	}

// 	err = tempHamt.Set(ctx, string(abr.ObjectId), cp.Transactions[i])
// 	if err != nil {
// 		return false, fmt.Errorf("error setting hamt: %v", err)
// 	}
// }
