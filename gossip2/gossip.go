package gossip2

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/quorumcontrol/chaintree/dag"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/namedlocker"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/storage"
)

func init() {
	cbornode.RegisterCborType(gossipMessage{})
}

type internalCtxKey int

const (
	ctxRequestKey internalCtxKey = iota
	ctxStartKey   internalCtxKey = iota
)

type objectID []byte
type messageID []byte
type transaction []byte
type transactionID []byte
type conflictSetID []byte
type tip []byte
type phase int8
type roundSignatures map[string]storedTransaction

const messageTypeGossip = "GOSSIP"

const (
	phasePrepare         phase = 0
	phaseTentativeCommit phase = 1
	phaseCommit          phase = 2
)

var (
	stateBucket = []byte("currentState")
)

func (pt tip) Equals(otherTip tip) bool {
	return bytes.Equal([]byte(pt), []byte(otherTip))
}

func (t transaction) ID() transactionID {
	return transactionID(crypto.Keccak256([]byte(t)))
}

type handler interface {
	DoRequest(dst *ecdsa.PublicKey, req *network.Request) (chan *network.Response, error)
	Push(dst *ecdsa.PublicKey, req *network.Request) error
	AssignHandler(requestType string, handlerFunc network.HandlerFunc) error
	Start()
	Stop()
}

type stateHandler func(ctx context.Context, group *consensus.Group, objectId, transaction, currentState []byte) (nextState []byte, accepted bool, err error)
type acceptedHandler func(ctx context.Context, group *consensus.Group, objectId, transaction, newState []byte) (err error)
type rejectedHandler func(ctx context.Context, group *consensus.Group, objectId, transaction, currentState []byte) (err error)

type currentState struct {
	objectID  objectID
	tip       tip
	round     int64
	signature consensus.Signature
}

type gossipMessage struct {
	objectID         objectID
	previousTip      tip
	newTip           tip
	transaction      transaction
	phase            phase
	prepareSignature consensus.Signature
	phaseSignatures  consensus.SignatureMap
	round            int64
}

func (gm *gossipMessage) toUnsignedStoredTransaction() *storedTransaction {
	return &storedTransaction{
		objectID:                  gm.objectID,
		previousTip:               gm.previousTip,
		transaction:               gm.transaction,
		newTip:                    gm.newTip,
		round:                     gm.round,
		prepareSignatures:         make(consensus.SignatureMap),
		tentativeCommitSignatures: make(consensus.SignatureMap),
	}
}

type storedTransaction struct {
	objectID                  objectID
	previousTip               tip
	newTip                    tip
	transaction               transaction
	round                     int64
	phase                     phase
	prepareSignatures         consensus.SignatureMap
	tentativeCommitSignatures consensus.SignatureMap
}

func (st *storedTransaction) ID() transactionID {
	return transactionToID(st.transaction, st.round)
}

func (st *storedTransaction) signPrepare(key *bls.SignKey) (*consensus.Signature, error) {
	return consensus.BlsSignBytes(st.msgToSignPrepare(), key)
}

func (st *storedTransaction) signTentativeCommit(key *bls.SignKey) (*consensus.Signature, error) {
	return consensus.BlsSignBytes(st.msgToSignTentativeCommit(), key)
}

func (st *storedTransaction) verifyPrepareSignatures(group *consensus.Group, sigs consensus.SignatureMap) (bool, error) {
	sig, err := group.CombineSignatures(sigs)
	if err != nil {
		return false, fmt.Errorf("error combining signatures: %v", err)
	}
	return group.VerifyAvailableSignatures(st.msgToSignPrepare(), sig)
}

func (st *storedTransaction) verifyTentativeCommitSignatures(group *consensus.Group, sigs consensus.SignatureMap) (bool, error) {
	sig, err := group.CombineSignatures(sigs)
	if err != nil {
		return false, fmt.Errorf("error combining signatures: %v", err)
	}
	return group.VerifyAvailableSignatures(st.msgToSignTentativeCommit(), sig)
}

func (st *storedTransaction) msgToSignPrepare() []byte {
	msg := append([]byte("PREPARE"), st.previousTip...)
	msg = append(msg, st.transaction...)
	msg = append(msg, st.newTip...)
	return append(msg, roundToBytes(st.round)...)
}

func (st *storedTransaction) msgToSignTentativeCommit() []byte {
	msg := append(st.objectID, st.newTip...)
	msg = append(msg, roundToBytes(st.round)...)
	return msg
}

// Gossiper is a node in a network (belonging to a group) that gossips around messages and comes to
// a consensus about what the current state of an object is.
type Gossiper struct {
	MessageHandler  handler
	ID              string
	SignKey         *bls.SignKey
	Group           *consensus.Group
	Storage         storage.Storage
	StateHandler    stateHandler
	AcceptedHandler acceptedHandler
	RejectedHandler rejectedHandler
	RoundLength     int
	Fanout          int
	locker          *namedlocker.NamedLocker
}

const (
	defaultRoundLength = 10
	defaultFanout      = 3
)

// Initialize sets up the storage and data structures of the gossiper
func (g *Gossiper) Initialize() {
	g.Storage.CreateBucketIfNotExists(stateBucket)
	g.locker = namedlocker.NewNamedLocker()
	if g.RoundLength == 0 {
		g.RoundLength = defaultRoundLength
	}
	if g.Fanout == 0 {
		g.Fanout = defaultFanout
	}
}

func (g *Gossiper) roundAt(t time.Time) int64 {
	return t.UTC().Unix() / int64(g.RoundLength)
}

func (g *Gossiper) handleIncomingRequest(ctx context.Context, req network.Request, respChan network.ResponseChan) error {
	ctx = context.WithValue(ctx, ctxRequestKey, req.Id)
	ctx = context.WithValue(ctx, ctxStartKey, time.Now())

	gm := &gossipMessage{}
	err := cbornode.DecodeInto(req.Payload, gm)
	if err != nil {
		return fmt.Errorf("error decoding: %v", err)
	}
	return g.handleGossip(ctx, gm)
}

// if the message round doesn't match, then look at the conflict set for the correct transaction to gossip
// otherwise, make sure that the message is valid (if it's new to us, we'll compare tips and run it through the state handler)
// if the message is new and a PREPARE message then we save the message to the conflic set, add our signature to the PREPARE and then gossip
// if the PREPARE message has 2/3 of signers then gossip a TENTATIVE_COMMIT message by wrapping up the prepare sigs into one and starting a new phase with own sig
// if the message is a TENTATIVE_COMMIT message and the proof is valid then add known sigs and gossip
// if the TENTATIVE_COMMIT message has 2/3 of signers, then COMMIT
func (g *Gossiper) handleGossip(ctx context.Context, msg *gossipMessage) error {
	// currentRound := g.roundAt(time.Now())
	currState, err := g.getCurrentState(msg.objectID)
	if err != nil {
		return fmt.Errorf("error getting current state: %v", err)
	}

	//TODO: if this is a commit message, then we can probably still accept it
	if !msg.previousTip.Equals(currState.tip) {
		return fmt.Errorf("msg tip did not match current stored tip current: %s, msg: %s", currState.tip, msg.previousTip)
	}

	switch msg.phase {
	case phasePrepare:
		if len(msg.phaseSignatures) > 0 {
			trans := msg.toUnsignedStoredTransaction()
			verified, err := trans.verifyPrepareSignatures(g.Group, msg.phaseSignatures)
			if err != nil {
				return fmt.Errorf("error verifying signatures: %v", err)
			}
			if !verified {
				return fmt.Errorf("error, invalid phaseSignatures")
			}
		}
		return g.handlePrepareMessage(ctx, msg)
	case phaseTentativeCommit:
		if len(msg.phaseSignatures) > 0 {
			trans := msg.toUnsignedStoredTransaction()
			verified, err := trans.verifyTentativeCommitSignatures(g.Group, msg.phaseSignatures)
			if err != nil {
				return fmt.Errorf("error verifying signatures: %v", err)
			}
			if !verified {
				return fmt.Errorf("error, invalid phaseSignatures")
			}
		}
		return g.handleTentativeCommitMessage(ctx, msg)
	default:
		return fmt.Errorf("unknown phase: %d", msg.phase)
	}
}

// if the message is new and a PREPARE message then we save the message to the conflic set, add our signature to the PREPARE and then gossip
// if the PREPARE message has 2/3 of signers then gossip a TENTATIVE_COMMIT message by wrapping up the prepare sigs into one and starting a new phase with own sig
func (g *Gossiper) handlePrepareMessage(ctx context.Context, msg *gossipMessage) error {
	if msg.phase != phasePrepare {
		return fmt.Errorf("incorrect phase: %d", msg.phase)
	}

	trans := msg.toUnsignedStoredTransaction()
	csID := msgToConflictSetID(msg)
	err := g.Storage.CreateBucketIfNotExists(csID)
	if err != nil {
		return fmt.Errorf("error creating bucket: %v", err)
	}

	g.locker.Lock(string(csID))
	defer g.locker.Unlock(string(csID))

	savedTrans, err := g.getTransaction(csID, trans.ID())
	if err != nil {
		return fmt.Errorf("error getting transaction: %v", err)
	}

	if savedTrans == nil {
		// new transaction
		currState, err := g.getCurrentState(msg.objectID)
		if err != nil {
			return fmt.Errorf("error getting current state: %v", err)
		}

		trans, err := g.handleNewTransaction(ctx, csID, currState, msg)
		if err != nil {
			return fmt.Errorf("error handling transaction: %v", err)
		}
		if trans != nil {
			savedTrans = trans
		}
	} else {
		if savedTrans.phase != phasePrepare {
			// drop the message
			return nil
		}
		savedTrans.prepareSignatures = savedTrans.prepareSignatures.Merge(msg.phaseSignatures)
		err = g.saveTransaction(csID, savedTrans)
		if err != nil {
			return fmt.Errorf("error saving transaction: %v", err)
		}
	}

	var newMessage *gossipMessage
	// do we have 2/3 of the prepare messges?
	if int64(len(savedTrans.prepareSignatures)) >= g.Group.SuperMajorityCount() {
		// if so, then change phase
		prepareSig, err := g.Group.CombineSignatures(savedTrans.prepareSignatures)
		if err != nil {
			return fmt.Errorf("error combining sigs: %v", err)
		}
		savedTrans.phase = phaseTentativeCommit
		ownSig, err := savedTrans.signTentativeCommit(g.SignKey)
		if err != nil {
			return fmt.Errorf("error signing: %v", err)
		}
		savedTrans.tentativeCommitSignatures[g.ID] = *ownSig

		err = g.saveTransaction(csID, savedTrans)
		if err != nil {
			return fmt.Errorf("error saving transaction: %v", err)
		}

		newMessage = &gossipMessage{
			objectID:         savedTrans.objectID,
			previousTip:      savedTrans.previousTip,
			newTip:           savedTrans.newTip,
			transaction:      savedTrans.transaction,
			phase:            phaseTentativeCommit,
			prepareSignature: *prepareSig,
			phaseSignatures:  savedTrans.tentativeCommitSignatures,
			round:            savedTrans.round,
		}
	} else {
		// else gossip new prepare message
		newMessage = &gossipMessage{
			objectID:        savedTrans.objectID,
			previousTip:     savedTrans.previousTip,
			newTip:          savedTrans.newTip,
			transaction:     savedTrans.transaction,
			phase:           phasePrepare,
			phaseSignatures: savedTrans.prepareSignatures,
			round:           savedTrans.round,
		}
	}

	err = g.doGossip(newMessage)
	if err != nil {
		return fmt.Errorf("error doing gossip: %v", err)
	}

	return nil
}

func (g *Gossiper) handleTentativeCommitMessage(ctx context.Context, msg *gossipMessage) error {
	if msg.phase != phaseTentativeCommit {
		return fmt.Errorf("incorrect phase: %d", msg.phase)
	}

	trans := msg.toUnsignedStoredTransaction()

	// if the prepare signature is not valid then drop message
	verified, err := g.Group.VerifySignature(trans.msgToSignPrepare(), &msg.prepareSignature)
	if err != nil {
		return fmt.Errorf("error verifying signature: %v", err)
	}
	if !verified {
		return fmt.Errorf("error, prepare statement not verified")
	}

	csID := msgToConflictSetID(msg)
	err = g.Storage.CreateBucketIfNotExists(csID)
	if err != nil {
		return fmt.Errorf("error creating bucket: %v", err)
	}

	g.locker.Lock(string(csID))
	defer g.locker.Unlock(string(csID))

	savedTrans, err := g.getTransaction(csID, trans.ID())
	if err != nil {
		return fmt.Errorf("error getting transaction: %v", err)
	}

	if savedTrans == nil {
		// new transaction
		currState, err := g.getCurrentState(msg.objectID)
		if err != nil {
			return fmt.Errorf("error getting current state: %v", err)
		}

		trans, err := g.handleNewTransaction(ctx, csID, currState, msg)
		if err != nil {
			return fmt.Errorf("error handling transaction: %v", err)
		}
		if trans == nil {
			return fmt.Errorf("error creating transaction: %v", err)
		} else {
			savedTrans = trans
		}
	} else {
		savedTrans.phase = phaseTentativeCommit
		savedTrans.tentativeCommitSignatures = savedTrans.tentativeCommitSignatures.Merge(msg.phaseSignatures)
		err = g.saveTransaction(csID, savedTrans)
		if err != nil {
			return fmt.Errorf("error saving transaction: %v", err)
		}
	}

	var newMessage *gossipMessage
	// do we have 2/3 of the prepare messges?
	if int64(len(savedTrans.prepareSignatures)) >= g.Group.SuperMajorityCount() {
		newSig, err := g.Group.CombineSignatures(msg.phaseSignatures)
		if err != nil {
			return fmt.Errorf("error combining sigs: %v", err)
		}
		// commit
		newState := &currentState{
			objectID:  savedTrans.objectID,
			tip:       savedTrans.newTip,
			round:     savedTrans.round,
			signature: *newSig,
		}
		err = g.setCurrentState(newState)
		if err != nil {
			return fmt.Errorf("error setting new state: %v", err)
		}
		defer g.locker.Delete(string(csID))
		//TODO: delete conflict set bucket

	}
	newMessage = &gossipMessage{
		objectID:        savedTrans.objectID,
		previousTip:     savedTrans.previousTip,
		newTip:          savedTrans.newTip,
		transaction:     savedTrans.transaction,
		phase:           phaseTentativeCommit,
		phaseSignatures: savedTrans.tentativeCommitSignatures,
		round:           savedTrans.round,
	}
	err = g.doGossip(newMessage)
	if err != nil {
		return fmt.Errorf("error doing gossip: %v", err)
	}

	return nil
}

func (g *Gossiper) doGossip(msg *gossipMessage) error {
	num := min(g.Fanout, len(g.Group.SortedMembers))
	for i := 0; i < num; i++ {
		req, err := network.BuildRequest(messageTypeGossip, msg)
		if err != nil {
			return fmt.Errorf("error building request: %v", err)
		}
		rn := g.Group.RandomMember()
		err = g.MessageHandler.Push(rn.DstKey.ToEcdsaPub(), req)
		if err != nil {
			return fmt.Errorf("error pushing message: %v", err)
		}
	}
	return nil
}

func (g *Gossiper) handleNewTransaction(ctx context.Context, csID conflictSetID, currState currentState, msg *gossipMessage) (*storedTransaction, error) {
	trans := msg.toUnsignedStoredTransaction()
	newTip, isAccepted, err := g.StateHandler(ctx, g.Group, trans.objectID, trans.transaction, currState.tip)
	if err != nil {
		return nil, fmt.Errorf("error calling state handler: %v", err)
	}

	if isAccepted {
		trans.newTip = newTip
		switch msg.phase {
		case phasePrepare:
			ownSig, err := trans.signPrepare(g.SignKey)
			if err != nil {
				return nil, fmt.Errorf("error signing: %v", err)
			}
			trans.prepareSignatures[g.ID] = *ownSig
			trans.prepareSignatures = msg.phaseSignatures.Merge(trans.prepareSignatures)
		case phaseTentativeCommit:
			ownSig, err := trans.signTentativeCommit(g.SignKey)
			if err != nil {
				return nil, fmt.Errorf("error signing: %v", err)
			}
			trans.tentativeCommitSignatures[g.ID] = *ownSig
			trans.tentativeCommitSignatures = msg.phaseSignatures.Merge(trans.tentativeCommitSignatures)
		default:
			return nil, fmt.Errorf("error, unkown phase: %d", msg.phase)
		}

		g.saveTransaction(csID, trans)
		return trans, nil
	}
	// the transaction wasn't accepted so just move on
	return nil, nil
}

func (g *Gossiper) getTransaction(csID conflictSetID, transID transactionID) (*storedTransaction, error) {
	transBytes, err := g.Storage.Get(csID, transID)
	if err != nil {
		return nil, fmt.Errorf("error getting transactions: %v", err)
	}

	if len(transBytes) == 0 {
		return nil, nil
	}

	trans := &storedTransaction{}
	err = cbornode.DecodeInto(transBytes, trans)
	if err != nil {
		return nil, fmt.Errorf("error decoding: %v", err)
	}
	return trans, nil
}

func (g *Gossiper) saveTransaction(csID conflictSetID, trans *storedTransaction) error {
	sw := dag.SafeWrap{}
	node := sw.WrapObject(trans)
	if sw.Err != nil {
		return fmt.Errorf("error wrapping object: %v", sw.Err)
	}

	return g.Storage.Set(csID, trans.ID(), node.RawData())
}

func transactionToID(trans transaction, round int64) transactionID {
	return crypto.Keccak256(append(trans, roundToBytes(round)...))
}

func (g *Gossiper) getCurrentState(objID objectID) (currState currentState, err error) {
	g.locker.RLock(string(objID))
	defer g.locker.RUnlock(string(objID))

	stateBytes, err := g.Storage.Get(stateBucket, objID)
	if err != nil {
		return currState, fmt.Errorf("error getting state bytes: %v", err)
	}

	if len(stateBytes) > 0 {
		err = cbornode.DecodeInto(stateBytes, &currState)
		if err != nil {
			return currState, fmt.Errorf("error decoding: %v", err)
		}
		return currState, nil
	}
	return currState, nil
}

func (g *Gossiper) setCurrentState(state *currentState) error {
	g.locker.Lock(string(state.objectID))
	defer g.locker.Unlock(string(state.objectID))

	sw := &dag.SafeWrap{}
	node := sw.WrapObject(state)
	if sw.Err != nil {
		return fmt.Errorf("error wraping: %v", sw.Err)
	}
	return g.Storage.Set(stateBucket, state.objectID, node.RawData())
}

func roundToBytes(round int64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(round))
	return b
}

func msgToConflictSetID(msg *gossipMessage) conflictSetID {
	return crypto.Keccak256(append(msg.objectID, msg.previousTip...))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
