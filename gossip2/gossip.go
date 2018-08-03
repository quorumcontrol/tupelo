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
	"github.com/ethereum/go-ethereum/log"

	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/namedlocker"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/storage"
)

func init() {
	cbornode.RegisterCborType(storedTransaction{})
	cbornode.RegisterCborType(CurrentState{})
	cbornode.RegisterCborType(GossipMessage{})
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

const MessageTypeGossip = "GOSSIP"

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

// CurrentState is the state of an object in the gossip system
// and its associated tip and signature
type CurrentState struct {
	ObjectID  objectID
	Tip       tip
	Round     int64
	Signature consensus.Signature
}

// GossipMessage is the message used for all phases of the consensus algorithm
type GossipMessage struct {
	ObjectID         objectID
	PreviousTip      tip
	NewTip           tip
	Transaction      transaction
	Phase            phase
	PrepareSignature consensus.Signature
	PhaseSignatures  consensus.SignatureMap
	Round            int64
}

func (gm *GossipMessage) toUnsignedStoredTransaction() *storedTransaction {
	return &storedTransaction{
		ObjectID:                  gm.ObjectID,
		PreviousTip:               gm.PreviousTip,
		Transaction:               gm.Transaction,
		NewTip:                    gm.NewTip,
		Round:                     gm.Round,
		PrepareSignatures:         make(consensus.SignatureMap),
		TentativeCommitSignatures: make(consensus.SignatureMap),
	}
}

type storedTransaction struct {
	ObjectID                  objectID
	PreviousTip               tip
	NewTip                    tip
	Transaction               transaction
	Round                     int64
	Phase                     phase
	PrepareSignatures         consensus.SignatureMap
	TentativeCommitSignatures consensus.SignatureMap
}

func (st *storedTransaction) ID() transactionID {
	return transactionToID(st.Transaction, st.Round)
}

func (st *storedTransaction) signPrepare(key *bls.SignKey) (*consensus.Signature, error) {
	return consensus.BlsSignBytes(st.msgToSignPrepare(), key)
}

func (st *storedTransaction) signTentativeCommit(key *bls.SignKey) (*consensus.Signature, error) {
	return consensus.BlsSignBytes(st.msgToSignTentativeCommit(), key)
}

func (st *storedTransaction) verifyPrepareSignatures(group *consensus.Group, sigs consensus.SignatureMap) (bool, error) {
	log.Debug("combinging sigs for verifyingPrepare")
	sig, err := group.CombineSignatures(sigs)
	if err != nil {
		return false, fmt.Errorf("error combining signatures: %v", err)
	}
	return group.VerifyAvailableSignatures(st.msgToSignPrepare(), sig)
}

func (st *storedTransaction) verifyTentativeCommitSignatures(group *consensus.Group, sigs consensus.SignatureMap) (bool, error) {
	log.Debug("combinging sigs for verifyingTenativeCommit")
	sig, err := group.CombineSignatures(sigs)
	if err != nil {
		return false, fmt.Errorf("error combining signatures: %v", err)
	}
	return group.VerifyAvailableSignatures(st.msgToSignTentativeCommit(), sig)
}

func (st *storedTransaction) msgToSignPrepare() []byte {
	msg := append([]byte("PREPARE"), st.PreviousTip...)
	msg = append(msg, st.Transaction...)
	msg = append(msg, st.NewTip...)
	return crypto.Keccak256(append(msg, roundToBytes(st.Round)...))
}

func (st *storedTransaction) msgToSignTentativeCommit() []byte {
	log.Debug("msgToSignTentativeCommit", "objectID", string(st.ObjectID), "newTip", string(st.NewTip), "round", st.Round)
	msg := append(st.ObjectID, st.NewTip...)
	msg = append(msg, roundToBytes(st.Round)...)
	return crypto.Keccak256(msg)
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
	started         bool
}

const (
	defaultRoundLength = 10
	defaultFanout      = 5
)

// Initialize sets up the storage and data structures of the gossiper
func (g *Gossiper) Initialize() {
	if g.ID == "" {
		g.ID = consensus.BlsVerKeyToAddress(g.SignKey.MustVerKey().Bytes()).String()
	}
	g.Storage.CreateBucketIfNotExists(stateBucket)
	g.MessageHandler.AssignHandler(MessageTypeGossip, g.handleIncomingRequest)
	g.locker = namedlocker.NewNamedLocker()
	if g.RoundLength == 0 {
		g.RoundLength = defaultRoundLength
	}
	if g.Fanout == 0 {
		g.Fanout = defaultFanout
	}
}

// Start starts the message handler and gossiper
func (g *Gossiper) Start() {
	if g.started {
		return
	}
	g.started = true
	g.MessageHandler.Start()
}

// Stop stops the message handler and gossiper
func (g *Gossiper) Stop() {
	if !g.started {
		return
	}
	g.started = false
	g.MessageHandler.Stop()
}

// RoundAt returns the round number for the time
func (g *Gossiper) RoundAt(t time.Time) int64 {
	return t.UTC().Unix() / int64(g.RoundLength)
}

func (g *Gossiper) handleIncomingRequest(ctx context.Context, req network.Request, respChan network.ResponseChan) error {
	ctx = context.WithValue(ctx, ctxRequestKey, req.Id)
	ctx = context.WithValue(ctx, ctxStartKey, time.Now())

	log.Debug("handleIncomingRequest", "g", g.ID, "uuid", ctx.Value(ctxRequestKey))

	gm := &GossipMessage{}
	err := cbornode.DecodeInto(req.Payload, gm)
	if err != nil {
		return fmt.Errorf("error decoding: %v", err)
	}
	return g.HandleGossip(ctx, gm)
}

// if the message round doesn't match, then look at the conflict set for the correct transaction to gossip
// otherwise, make sure that the message is valid (if it's new to us, we'll compare tips and run it through the state handler)
// if the message is new and a PREPARE message then we save the message to the conflic set, add our signature to the PREPARE and then gossip
// if the PREPARE message has 2/3 of signers then gossip a TENTATIVE_COMMIT message by wrapping up the prepare sigs into one and starting a new phase with own sig
// if the message is a TENTATIVE_COMMIT message and the proof is valid then add known sigs and gossip
// if the TENTATIVE_COMMIT message has 2/3 of signers, then COMMIT
func (g *Gossiper) HandleGossip(ctx context.Context, msg *GossipMessage) error {
	log.Debug("handleGossip", "g", g.ID, "uuid", ctx.Value(ctxRequestKey), "sigCount", len(msg.PhaseSignatures), "phase", msg.Phase)

	currentRound := g.RoundAt(time.Now())

	if msg.Round > currentRound+1 {
		log.Info("dropping message from the future", "g", g.ID, "uuid", ctx.Value(ctxRequestKey), "msgRound", msg.Round, "round", currentRound, "phase", msg.Phase)
		return nil
	}
	if msg.Round < currentRound-1 {
		log.Info("dropping message from the past", "g", g.ID, "uuid", ctx.Value(ctxRequestKey), "msgRound", msg.Round, "round", currentRound, "sigCount", len(msg.PhaseSignatures), "phase", msg.Phase)
		return nil
	}

	switch msg.Phase {
	case phasePrepare:
		return g.handlePrepareMessage(ctx, msg)
	case phaseTentativeCommit:
		if len(msg.PhaseSignatures) > 0 {
			trans := msg.toUnsignedStoredTransaction()
			verified, err := trans.verifyTentativeCommitSignatures(g.Group, msg.PhaseSignatures)
			if err != nil {
				return fmt.Errorf("error verifying signatures: %v", err)
			}
			if !verified {
				return fmt.Errorf("error, invalid phaseSignatures in phaseTenativeCommit")
			}
		}
		return g.handleTentativeCommitMessage(ctx, msg)
	default:
		return fmt.Errorf("unknown phase: %d", msg.Phase)
	}
}

// if the message is new and a PREPARE message then we save the message to the conflic set, add our signature to the PREPARE and then gossip
// if the PREPARE message has 2/3 of signers then gossip a TENTATIVE_COMMIT message by wrapping up the prepare sigs into one and starting a new phase with own sig
func (g *Gossiper) handlePrepareMessage(ctx context.Context, msg *GossipMessage) error {
	log.Debug("handlePrepareMessage", "g", g.ID, "uuid", ctx.Value(ctxRequestKey), "sigCount", len(msg.PhaseSignatures))

	if msg.Phase != phasePrepare {
		return fmt.Errorf("incorrect phase: %d", msg.Phase)
	}

	trans := msg.toUnsignedStoredTransaction()
	csID := msgToConflictSetID(msg)

	g.locker.Lock(string(csID))
	defer g.locker.Unlock(string(csID))
	err := g.Storage.CreateBucketIfNotExists(csID)
	if err != nil {
		return fmt.Errorf("error creating bucket: %v", err)
	}

	savedTrans, err := g.getTransaction(csID, trans.ID())
	if err != nil {
		return fmt.Errorf("error getting transaction: %v", err)
	}

	if savedTrans != nil && savedTrans.Phase != phasePrepare {
		// just drop an old prepare message
		return nil
	}

	newSigs := msg.PhaseSignatures
	if savedTrans != nil {
		newSigs = msg.PhaseSignatures.Subtract(savedTrans.PrepareSignatures)
	}

	if len(newSigs) > 0 {
		trans := msg.toUnsignedStoredTransaction()
		verified, err := trans.verifyPrepareSignatures(g.Group, newSigs)
		if err != nil {
			return fmt.Errorf("error verifying signatures: %v", err)
		}
		if !verified {
			return fmt.Errorf("error, invalid phaseSignatures in prepare message")
		}
	}

	if savedTrans == nil {
		log.Debug("handlePrepareMessage - new transaction", "g", g.ID, "uuid", ctx.Value(ctxRequestKey))

		// new transaction
		currState, err := g.GetCurrentState(msg.ObjectID)
		if err != nil {
			return fmt.Errorf("error getting current state: %v", err)
		}

		trans, err := g.handleNewTransaction(ctx, csID, currState, msg)
		if err != nil {
			return fmt.Errorf("error handling transaction: %v", err)
		}
		if trans != nil {
			log.Debug("handlePrepareMessage - saved new transaction", "g", g.ID, "uuid", ctx.Value(ctxRequestKey), "sigCount", len(trans.PrepareSignatures))
			savedTrans = trans
		} else {
			return fmt.Errorf("error creating new transaction: %v", err)
		}
	} else {
		log.Debug("handlePrepareMessag - existing transaction", "g", g.ID, "uuid", ctx.Value(ctxRequestKey))
		savedTrans.PrepareSignatures = savedTrans.PrepareSignatures.Merge(newSigs)
		err = g.saveTransaction(csID, savedTrans)
		if err != nil {
			return fmt.Errorf("error saving transaction: %v", err)
		}
	}

	var newMessage *GossipMessage
	// do we have 2/3 of the prepare messges?
	if int64(len(savedTrans.PrepareSignatures)) >= g.Group.SuperMajorityCount() {
		log.Debug("handlePrepareMessag - super majority prepared", "g", g.ID, "uuid", ctx.Value(ctxRequestKey))

		// if so, then change phase
		prepareSig, err := g.Group.CombineSignatures(savedTrans.PrepareSignatures)
		if err != nil {
			return fmt.Errorf("error combining sigs: %v", err)
		}
		savedTrans.Phase = phaseTentativeCommit
		ownSig, err := savedTrans.signTentativeCommit(g.SignKey)
		if err != nil {
			return fmt.Errorf("error signing: %v", err)
		}
		savedTrans.TentativeCommitSignatures[g.ID] = *ownSig

		err = g.saveTransaction(csID, savedTrans)
		if err != nil {
			return fmt.Errorf("error saving transaction: %v", err)
		}

		newMessage = &GossipMessage{
			ObjectID:         savedTrans.ObjectID,
			PreviousTip:      savedTrans.PreviousTip,
			NewTip:           savedTrans.NewTip,
			Transaction:      savedTrans.Transaction,
			Phase:            phaseTentativeCommit,
			PrepareSignature: *prepareSig,
			PhaseSignatures:  savedTrans.TentativeCommitSignatures,
			Round:            savedTrans.Round,
		}
	} else {
		log.Debug("handlePrepareMessag - re-gossipping", "g", g.ID, "uuid", ctx.Value(ctxRequestKey), "sigCount", len(savedTrans.PrepareSignatures))

		// else gossip new prepare message
		newMessage = &GossipMessage{
			ObjectID:        savedTrans.ObjectID,
			PreviousTip:     savedTrans.PreviousTip,
			NewTip:          savedTrans.NewTip,
			Transaction:     savedTrans.Transaction,
			Phase:           phasePrepare,
			PhaseSignatures: savedTrans.PrepareSignatures,
			Round:           savedTrans.Round,
		}
	}

	if len(g.Group.SortedMembers) == 1 {
		go func() {
			g.HandleGossip(ctx, newMessage)
		}()
	} else {
		g.doGossip(ctx, newMessage)
	}

	return nil
}

func (g *Gossiper) handleTentativeCommitMessage(ctx context.Context, msg *GossipMessage) error {
	log.Debug("handleTentativeCommitMessage", "g", g.ID, "uuid", ctx.Value(ctxRequestKey))

	if msg.Phase != phaseTentativeCommit {
		return fmt.Errorf("incorrect phase: %d", msg.Phase)
	}

	trans := msg.toUnsignedStoredTransaction()

	// if the prepare signature is not valid then drop message
	log.Trace("prepare sig", "sig", msg.PrepareSignature)
	verified, err := g.Group.VerifySignature(trans.msgToSignPrepare(), &msg.PrepareSignature)
	if err != nil {
		return fmt.Errorf("error verifying signature: %v", err)
	}
	if !verified {
		return fmt.Errorf("error, prepare statement not verified")
	}

	csID := msgToConflictSetID(msg)

	g.locker.Lock(string(csID))
	defer g.locker.Unlock(string(csID))
	err = g.Storage.CreateBucketIfNotExists(csID)
	if err != nil {
		return fmt.Errorf("error creating bucket: %v", err)
	}

	savedTrans, err := g.getTransaction(csID, trans.ID())
	if err != nil {
		return fmt.Errorf("error getting transaction: %v", err)
	}

	newSigs := msg.PhaseSignatures
	if savedTrans != nil {
		newSigs = msg.PhaseSignatures.Subtract(savedTrans.TentativeCommitSignatures)
	}

	if len(newSigs) > 0 {
		trans := msg.toUnsignedStoredTransaction()
		verified, err := trans.verifyTentativeCommitSignatures(g.Group, newSigs)
		if err != nil {
			return fmt.Errorf("error verifying signatures: %v", err)
		}
		if !verified {
			return fmt.Errorf("error, invalid phaseSignatures in tentativeCommit message")
		}
	}

	if savedTrans == nil {
		// new transaction
		currState, err := g.GetCurrentState(msg.ObjectID)
		if err != nil {
			return fmt.Errorf("error getting current state: %v", err)
		}

		trans, err := g.handleNewTransaction(ctx, csID, currState, msg)
		if err != nil {
			return fmt.Errorf("error handling transaction: %v", err)
		}
		if trans == nil {
			return fmt.Errorf("error creating transaction: %v", err)
		}
		savedTrans = trans
	} else {
		if len(newSigs) > 0 {
			savedTrans.Phase = phaseTentativeCommit
			savedTrans.TentativeCommitSignatures = savedTrans.TentativeCommitSignatures.Merge(newSigs)
			err = g.saveTransaction(csID, savedTrans)
			if err != nil {
				return fmt.Errorf("error saving transaction: %v", err)
			}
		}
	}

	var newMessage *GossipMessage
	// do we have 2/3 of the TentativeCommit messges?
	if int64(len(savedTrans.TentativeCommitSignatures)) >= g.Group.SuperMajorityCount() && savedTrans.Phase != phaseCommit {
		newSig, err := g.Group.CombineSignatures(savedTrans.TentativeCommitSignatures)
		if err != nil {
			return fmt.Errorf("error combining sigs: %v", err)
		}
		// commit
		newState := &CurrentState{
			ObjectID:  savedTrans.ObjectID,
			Tip:       savedTrans.NewTip,
			Round:     savedTrans.Round,
			Signature: *newSig,
		}
		log.Debug("---------COMMIT", "g", g.ID, "uuid", ctx.Value(ctxRequestKey))
		err = g.setCurrentState(newState)
		if err != nil {
			return fmt.Errorf("error setting new state: %v", err)
		}
		savedTrans.Phase = phaseCommit
		err = g.saveTransaction(csID, savedTrans)
		if err != nil {
			return fmt.Errorf("error saving transaction: %v", err)
		}

		if g.AcceptedHandler != nil {
			err = g.AcceptedHandler(ctx, g.Group, newState.ObjectID, savedTrans.Transaction, newState.Tip)
			if err != nil {
				log.Error("error calling accepted", "err", err)
				return fmt.Errorf("error calling accepted: %v", err)
			}
		}

		//TODO: when do we delete the transaction
	}

	if len(savedTrans.TentativeCommitSignatures) == len(g.Group.SortedMembers) {
		log.Info("saturated the network, stopping gossip", "g", g.ID, "uuid", ctx.Value(ctxRequestKey))
		return nil
	}
	newMessage = &GossipMessage{
		ObjectID:         savedTrans.ObjectID,
		PreviousTip:      savedTrans.PreviousTip,
		NewTip:           savedTrans.NewTip,
		Transaction:      savedTrans.Transaction,
		Phase:            phaseTentativeCommit,
		PrepareSignature: msg.PrepareSignature,
		PhaseSignatures:  savedTrans.TentativeCommitSignatures,
		Round:            savedTrans.Round,
	}

	g.doGossip(ctx, newMessage)

	return nil
}

func (g *Gossiper) doGossip(ctx context.Context, msg *GossipMessage) {
	go func(ctx context.Context, msg *GossipMessage) {
		num := min(g.Fanout, len(g.Group.SortedMembers)-1)
		log.Debug("gossipping", "g", g.ID, "uuid", ctx.Value(ctxRequestKey), "num", num, "phase", msg.Phase, "sigCount", len(msg.PhaseSignatures))

		for i := 0; i < num; i++ {
			req, err := network.BuildRequest(MessageTypeGossip, msg)
			if err != nil {
				log.Error("error building request", "err", err)
			}
			var rn *consensus.RemoteNode
			for rn == nil || rn.Id == g.ID {
				rn = g.Group.RandomMember()
			}
			log.Debug("one gossip", "g", g.ID, "uuid", ctx.Value(ctxRequestKey), "dst", consensus.PublicKeyToAddr(&rn.VerKey), "phase", msg.Phase, "sigCount", len(msg.PhaseSignatures))

			err = g.MessageHandler.Push(rn.DstKey.ToEcdsaPub(), req)
			if err != nil {
				log.Error("error pushing", "err", err)
			}
		}
	}(ctx, msg)
}

func (g *Gossiper) handleNewTransaction(ctx context.Context, csID conflictSetID, currState CurrentState, msg *GossipMessage) (*storedTransaction, error) {
	log.Debug("handleNewTransaction", "g", g.ID, "uuid", ctx.Value(ctxRequestKey))
	trans := msg.toUnsignedStoredTransaction()
	newTip, isAccepted, err := g.StateHandler(ctx, g.Group, trans.ObjectID, trans.Transaction, currState.Tip)
	if err != nil {
		return nil, fmt.Errorf("error calling state handler: %v", err)
	}

	if isAccepted {
		log.Debug("handleNewTransaction - accepted", "g", g.ID, "uuid", ctx.Value(ctxRequestKey))

		trans.NewTip = newTip
		switch msg.Phase {
		case phasePrepare:
			ownSig, err := trans.signPrepare(g.SignKey)
			if err != nil {
				return nil, fmt.Errorf("error signing: %v", err)
			}
			trans.PrepareSignatures[g.ID] = *ownSig
			trans.PrepareSignatures = msg.PhaseSignatures.Merge(trans.PrepareSignatures)
		case phaseTentativeCommit:
			ownSig, err := trans.signTentativeCommit(g.SignKey)
			if err != nil {
				return nil, fmt.Errorf("error signing: %v", err)
			}
			trans.TentativeCommitSignatures[g.ID] = *ownSig
			trans.TentativeCommitSignatures = msg.PhaseSignatures.Merge(trans.TentativeCommitSignatures)
		default:
			return nil, fmt.Errorf("error, unkown phase: %d", msg.Phase)
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

func (g *Gossiper) GetCurrentState(objID objectID) (currState CurrentState, err error) {
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

func (g *Gossiper) setCurrentState(state *CurrentState) error {
	g.locker.Lock(string(state.ObjectID))
	defer g.locker.Unlock(string(state.ObjectID))

	sw := &dag.SafeWrap{}
	node := sw.WrapObject(state)
	if sw.Err != nil {
		return fmt.Errorf("error wraping: %v", sw.Err)
	}
	return g.Storage.Set(stateBucket, state.ObjectID, node.RawData())
}

func roundToBytes(round int64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(round))
	return b
}

func msgToConflictSetID(msg *GossipMessage) conflictSetID {
	return crypto.Keccak256(append(msg.ObjectID, msg.PreviousTip...))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func elapsedTime(ctx context.Context) time.Duration {
	start := ctx.Value(ctxStartKey)
	if start == nil {
		start = 0
	}
	return time.Now().Sub(start.(time.Time))
}
