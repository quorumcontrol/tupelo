package gossip

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/namedlocker"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/stack"
	"github.com/quorumcontrol/storage"
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

const MessageTypeGossip = "GOSSIP"

const (
	phasePrepare phase = iota
	phaseTentativeCommit
	phaseCommit
)

var (
	stateBucket = []byte("currentState")
)

func (pt tip) Equals(otherTip tip) bool {
	return bytes.Equal([]byte(pt), []byte(otherTip))
}

type handler interface {
	DoRequest(dst *ecdsa.PublicKey, req *network.Request) (chan *network.Response, error)
	Push(dst *ecdsa.PublicKey, req *network.Request) error
	AssignHandler(requestType string, handlerFunc network.HandlerFunc) error
	Start()
	Stop()
}

// StateTransaction is what's passed to accepted
// handlers after the transaction is accepted
type StateTransaction struct {
	Group         *consensus.NotaryGroup
	Round         int64
	ObjectID      []byte
	Transaction   []byte
	TransactionID []byte
	State         []byte
}

type stateHandler func(ctx context.Context, inProgressTrans StateTransaction) (nextState []byte, accepted bool, err error)
type acceptedHandler func(ctx context.Context, acceptedTrans StateTransaction) (err error)
type roundHandler func(ctx context.Context, round int64)

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

func (st *storedTransaction) verifyPrepareSignatures(group *consensus.NotaryGroup, sigs consensus.SignatureMap) (bool, error) {
	log.Trace("combinging sigs for verifyingPrepare")
	sig, err := group.CombineSignatures(st.Round, sigs)
	if err != nil {
		return false, fmt.Errorf("error combining signatures: %v", err)
	}
	return group.VerifyAvailableSignatures(st.Round, st.msgToSignPrepare(), sig)
}

func (st *storedTransaction) verifyTentativeCommitSignatures(group *consensus.NotaryGroup, sigs consensus.SignatureMap) (bool, error) {
	log.Trace("combinging sigs for verifyingTenativeCommit")
	sig, err := group.CombineSignatures(st.Round, sigs)
	if err != nil {
		return false, fmt.Errorf("error combining signatures: %v", err)
	}
	return group.VerifyAvailableSignatures(st.Round, st.msgToSignTentativeCommit(), sig)
}

func (st *storedTransaction) msgToSignPrepare() []byte {
	msg := append([]byte("PREPARE"), st.PreviousTip...)
	msg = append(msg, st.Transaction...)
	msg = append(msg, st.NewTip...)
	return crypto.Keccak256(append(msg, roundToBytes(st.Round)...))
}

func (st *storedTransaction) msgToSignTentativeCommit() []byte {
	log.Trace("msgToSignTentativeCommit", "objectID", string(st.ObjectID), "newTip", string(st.NewTip), "round", st.Round)
	msg := append(st.ObjectID, st.NewTip...)
	msg = append(msg, roundToBytes(st.Round)...)
	return crypto.Keccak256(msg)
}

func (st *storedTransaction) toGossipMessage(p phase) *GossipMessage {
	newMessage := &GossipMessage{
		ObjectID:    st.ObjectID,
		PreviousTip: st.PreviousTip,
		NewTip:      st.NewTip,
		Transaction: st.Transaction,
		Round:       st.Round,
		Phase:       p,
	}

	switch p {
	case phasePrepare:
		newMessage.PhaseSignatures = st.PrepareSignatures
	case phaseTentativeCommit:
		newMessage.PhaseSignatures = st.TentativeCommitSignatures
	}

	return newMessage
}

// Gossiper is a node in a network (belonging to a group) that gossips around messages and comes to
// a consensus about what the current state of an object is.
type Gossiper struct {
	MessageHandler  handler
	ID              string
	SignKey         *bls.SignKey
	VerKey          consensus.PublicKey
	Group           *consensus.NotaryGroup
	Storage         storage.Storage
	StateHandler    stateHandler
	AcceptedHandler acceptedHandler
	Fanout          int
	locker          *namedlocker.NamedLocker
	started         bool
	stacks          *sync.Map
	stackPokeChan   chan *poker
	pokeStopChan    chan bool
	roundHandlers   []roundHandler
	roundStopChan   chan bool
}

const (
	defaultFanout = 5
)

// Initialize sets up the storage and data structures of the gossiper
func (g *Gossiper) Initialize() {
	if g.ID == "" {
		g.ID = consensus.BlsVerKeyToAddress(g.SignKey.MustVerKey().Bytes()).String()
	}
	g.VerKey = consensus.BlsKeyToPublicKey(g.SignKey.MustVerKey())
	g.Storage.CreateBucketIfNotExists(stateBucket)
	g.MessageHandler.AssignHandler(MessageTypeGossip, g.handleIncomingRequest)
	g.locker = namedlocker.NewNamedLocker()
	if g.Fanout == 0 {
		g.Fanout = defaultFanout
	}
	g.stacks = new(sync.Map)
	g.stackPokeChan = make(chan *poker)
	g.pokeStopChan = make(chan bool, concurrency)
	g.roundStopChan = make(chan bool, 1)
}

const concurrency = 10

// Start starts the message handler and gossiper
func (g *Gossiper) Start() {
	if g.started {
		return
	}
	for i := 0; i < concurrency; i++ {
		go g.pokeWorker(g.stackPokeChan, g.pokeStopChan)
	}
	g.started = true
	g.MessageHandler.Start()
	go g.roundHandler()
}

// Stop stops the message handler and gossiper
func (g *Gossiper) Stop() {
	if !g.started {
		return
	}
	g.started = false
	g.MessageHandler.Stop()
	g.roundStopChan <- true
	for i := 0; i < concurrency; i++ {
		g.pokeStopChan <- true
	}
}

type poker struct {
	stack *stack.Stack
	csID  conflictSetID
	ctx   context.Context
}

// AddRoundHandler adds a function that will be called
// every time a round changes, with the current round
func (g *Gossiper) AddRoundHandler(f roundHandler) {
	g.locker.Lock("roundHandlers")
	defer g.locker.Unlock("roundHandlers")
	g.roundHandlers = append(g.roundHandlers, f)
}

func (g *Gossiper) roundHandler() {
	nextRound := make(chan time.Time, 1)
	// don't immediately call the round handler at start, wait for the next
	// round to actually happen
	currRound := g.Group.RoundAt(time.Now())
	nextRoundAt := (currRound + 1) * int64(g.Group.RoundLength)
	nextAt := time.Unix(nextRoundAt, 0)
	nextRound <- <-time.After(nextAt.Sub(time.Now()))
	for {
		select {
		case <-g.roundStopChan:
			return
		case sent := <-nextRound:
			g.locker.RLock("roundHandlers")
			currRound := g.Group.RoundAt(sent)
			for _, handler := range g.roundHandlers {
				go handler(context.TODO(), currRound)
			}
			g.locker.RUnlock("roundHandlers")
			nextRoundAt := (currRound + 1) * int64(g.Group.RoundLength)
			nextAt := time.Unix(nextRoundAt, 0)
			diff := nextAt.Sub(time.Now())
			// in case we get behind on the rounds, we still call every round
			// if we're behind, we'll catch up
			if diff > 0 {
				nextRound <- <-time.After(diff)
			} else {
				nextRound <- nextAt
			}
		}
	}
}

func (g *Gossiper) pokeWorker(incoming <-chan *poker, stopChan <-chan bool) {
	for {
		select {
		case poke, ok := <-incoming:
			if !ok {
				return
			}
			g.locker.Lock(string(poke.csID))
			gm := poke.stack.Pop()
			if gm != nil {
				msg := gm.(*GossipMessage)
				err := g.HandleGossip(poke.ctx, msg)
				if err != nil {
					log.Error("error handling gossip", "g", g.ID, "uuid", poke.ctx.Value(ctxRequestKey), "err", err)
				}
			}
			g.locker.Unlock(string(poke.csID))
		case <-stopChan:
			return
		}

	}
}

// RoundAt returns the round number for the time
func (g *Gossiper) RoundAt(t time.Time) int64 {
	return g.Group.RoundAt(t)
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
	csID := msgToConflictSetID(gm)

	// We want to process messages in a LIFO instead of a FIFO order because later messages will have more signatures on them
	csStack, _ := g.stacks.LoadOrStore(string(csID), stack.NewStack())
	csStack.(*stack.Stack).Push(gm)
	poke := &poker{
		stack: csStack.(*stack.Stack),
		csID:  csID,
		ctx:   ctx,
	}

	g.stackPokeChan <- poke

	respChan <- nil
	return err
}

func (g *Gossiper) clearPendingMessages(csID conflictSetID) {
	csStack, _ := g.stacks.LoadOrStore(string(csID), stack.NewStack())
	csStack.(*stack.Stack).Clear()
}

// if the message round doesn't match, then look at the conflict set for the correct transaction to gossip
// otherwise, make sure that the message is valid (if it's new to us, we'll compare tips and run it through the state handler)
// if the message is new and a PREPARE message then we save the message to the conflict set, add our signature to the PREPARE and then gossip
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
		return g.handleTentativeCommitMessage(ctx, msg)
	case phaseCommit:
		return fmt.Errorf("ignoring gossip of commit phase message")
	default:
		return fmt.Errorf("unknown phase: %d", msg.Phase)
	}
}

// if the message is new and a PREPARE message then we save the message to the conflic set, add our signature to the PREPARE and then gossip
// if the PREPARE message has 2/3 of signers then gossip a TENTATIVE_COMMIT message by wrapping up the prepare sigs into one and starting a new phase with own sig
func (g *Gossiper) handlePrepareMessage(ctx context.Context, msg *GossipMessage) error {
	log.Debug("handlePrepareMessage", "g", g.ID, "objid", string(msg.ObjectID), "uuid", ctx.Value(ctxRequestKey), "round", msg.Round, "sigCount", len(msg.PhaseSignatures))

	if msg.Phase != phasePrepare {
		return fmt.Errorf("incorrect phase: %d", msg.Phase)
	}

	csID, savedTrans, err := g.conflictSetTransactionFromMessage(msg)
	if err != nil {
		return err
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
		trans, err := g.handleNewTransaction(ctx, csID, msg)
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

	roundInfo, err := g.Group.MostRecentRoundInfo(msg.Round)
	if roundInfo == nil || err != nil {
		return fmt.Errorf("error getting round info: %v", err)
	}
	// do we have 2/3 of the prepare messges?
	if int64(len(savedTrans.PrepareSignatures)) >= roundInfo.SuperMajorityCount() {
		log.Debug("handlePrepareMessag - super majority prepared", "g", g.ID, "uuid", ctx.Value(ctxRequestKey))

		// if so, then change phase
		prepareSig, err := g.Group.CombineSignatures(msg.Round, savedTrans.PrepareSignatures)
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
		log.Debug("handlePrepareMessage - resending TentativeCommitMessage", "g", g.ID, "uuid", ctx.Value(ctxRequestKey), "sigCount", len(savedTrans.TentativeCommitSignatures), "sigs", sigMapToString(savedTrans.TentativeCommitSignatures))

		newMessage = savedTrans.toGossipMessage(phaseTentativeCommit)
		newMessage.PrepareSignature = *prepareSig
	} else {
		log.Debug("handlePrepareMessage - re-gossipping", "g", g.ID, "uuid", ctx.Value(ctxRequestKey), "sigCount", len(savedTrans.PrepareSignatures), "sigs", sigMapToString(savedTrans.PrepareSignatures))

		// else gossip new prepare message
		newMessage = savedTrans.toGossipMessage(phasePrepare)
	}

	// If there is only one signer in the notary group, we can just process the message ourselves
	// since we can't gossip.
	if len(roundInfo.Signers) == 1 {
		go func() {
			g.HandleGossip(ctx, newMessage)
		}()
	} else {
		g.doGossip(ctx, newMessage)
	}

	return nil
}

func (g *Gossiper) conflictSetTransactionFromMessage(msg *GossipMessage) (conflictSetID, *storedTransaction, error) {
	trans := msg.toUnsignedStoredTransaction()
	csID := msgToConflictSetID(msg)

	err := g.Storage.CreateBucketIfNotExists(csID)
	if err != nil {
		return csID, nil, fmt.Errorf("error creating bucket: %v", err)
	}

	savedTrans, err := g.getTransaction(csID, trans.ID())
	if err != nil {
		return csID, nil, fmt.Errorf("error getting transaction: %v", err)
	}

	return csID, savedTrans, nil
}

func (g *Gossiper) handleTentativeCommitMessage(ctx context.Context, msg *GossipMessage) error {
	log.Debug("handleTentativeCommitMessage", "g", g.ID, "uuid", ctx.Value(ctxRequestKey))

	if msg.Phase != phaseTentativeCommit {
		return fmt.Errorf("incorrect phase: %d", msg.Phase)
	}

	trans := msg.toUnsignedStoredTransaction()

	// if the prepare signature is not valid then drop message
	log.Trace("prepare sig", "sig", msg.PrepareSignature)
	verified, err := g.Group.VerifySignature(msg.Round, trans.msgToSignPrepare(), &msg.PrepareSignature)
	if err != nil {
		return fmt.Errorf("error verifying signature: %v", err)
	}
	if !verified {
		return fmt.Errorf("error, prepare statement not verified")
	}

	csID, savedTrans, err := g.conflictSetTransactionFromMessage(msg)
	if err != nil {
		return err
	}

	newSigs := msg.PhaseSignatures
	if savedTrans != nil {
		_, hasOwnSignature := savedTrans.TentativeCommitSignatures[g.ID]
		if !hasOwnSignature {
			ownSig, err := trans.signTentativeCommit(g.SignKey)
			if err != nil {
				return fmt.Errorf("error signing: %v", err)
			}
			log.Debug("tentativeCommit - adding own signature", "g", g.ID, "uuid", ctx.Value(ctxRequestKey))
			savedTrans.TentativeCommitSignatures[g.ID] = *ownSig
		}
		log.Debug("saved trans, getting new sigs", "g", g.ID, "uuid", ctx.Value(ctxRequestKey), "savedSigs", sigMapToString(savedTrans.TentativeCommitSignatures), "msgSigs", sigMapToString(msg.PhaseSignatures))
		newSigs = msg.PhaseSignatures.Subtract(savedTrans.TentativeCommitSignatures)
	}

	if len(newSigs) > 0 {
		log.Debug("new signatures", "g", g.ID, "uuid", ctx.Value(ctxRequestKey), "sigCount", len(newSigs), "sigs", sigMapToString(newSigs))
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
		trans, err := g.handleNewTransaction(ctx, csID, msg)
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

	roundInfo, err := g.Group.MostRecentRoundInfo(msg.Round)
	if roundInfo == nil || err != nil {
		return fmt.Errorf("error getting round info: %v", err)
	}
	var newMessage *GossipMessage
	// do we have 2/3 of the TentativeCommit messges?
	if int64(len(savedTrans.TentativeCommitSignatures)) >= roundInfo.SuperMajorityCount() && savedTrans.Phase != phaseCommit {
		newSig, err := g.Group.CombineSignatures(msg.Round, savedTrans.TentativeCommitSignatures)
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
			err = g.AcceptedHandler(ctx, StateTransaction{
				Group:         g.Group,
				Round:         savedTrans.Round,
				ObjectID:      newState.ObjectID,
				Transaction:   savedTrans.Transaction,
				TransactionID: savedTrans.ID(),
				State:         newState.Tip,
			})
			if err != nil {
				log.Error("error calling accepted", "err", err)
				return fmt.Errorf("error calling accepted: %v", err)
			}
		}
	}

	newMessage = savedTrans.toGossipMessage(phaseTentativeCommit)
	newMessage.PrepareSignature = msg.PrepareSignature

	if savedTrans.Phase == phaseCommit {
		log.Debug("clearing pending messages", "g", g.ID, "uuid", ctx.Value(ctxRequestKey), "csId", string(csID))
		// clear out any pending messages if this is already committed and only gossip the commit ones
		g.clearPendingMessages(csID)
	}

	// Skip gossiping if this is commit phase and we already have 100% sigs
	if savedTrans.Phase == phaseCommit && len(savedTrans.TentativeCommitSignatures) >= len(roundInfo.Signers) {
		return nil
	}

	g.doGossip(ctx, newMessage)

	return nil
}

func sigMapToString(sigMap consensus.SignatureMap) string {
	haveSigs := ""
	for id := range sigMap {
		haveSigs += id + ", "
	}
	return haveSigs
}

func (g *Gossiper) doGossip(ctx context.Context, msg *GossipMessage) { //TODO: should this have errors?
	roundInfo, err := g.Group.MostRecentRoundInfo(msg.Round)
	if roundInfo == nil || err != nil {
		panic(fmt.Sprintf("should never happen: roundInfo problem: %v", err))
	}
	num := min(g.Fanout, len(roundInfo.Signers)-1)
	log.Debug("gossipping", "g", g.ID, "uuid", ctx.Value(ctxRequestKey), "num", num, "phase", msg.Phase, "sigCount", len(msg.PhaseSignatures), "sigs", sigMapToString(msg.PhaseSignatures))

	gossipList := map[string]bool{g.ID: true}

	for i := 0; i < num; i++ {
		req, err := network.BuildRequest(MessageTypeGossip, msg)
		if err != nil {
			log.Error("error building request", "err", err)
		}
		var rn *consensus.RemoteNode
		alreadyOnList := true
		for alreadyOnList {
			rn = roundInfo.RandomMember()
			log.Trace("rn search", "g", g.ID, "rn", rn.Id, "already", gossipList)
			_, alreadyOnList = gossipList[rn.Id]
			if !alreadyOnList {
				gossipList[rn.Id] = true
			}
		}
		log.Debug("one gossip", "g", g.ID, "uuid", ctx.Value(ctxRequestKey), "dst", consensus.PublicKeyToAddr(&rn.VerKey), "phase", msg.Phase, "sigCount", len(msg.PhaseSignatures))

		err = g.MessageHandler.Push(rn.DstKey.ToEcdsaPub(), req)
		if err != nil {
			log.Error("error pushing", "err", err)
		}
	}
}

func (g *Gossiper) handleNewTransaction(ctx context.Context, csID conflictSetID, msg *GossipMessage) (*storedTransaction, error) {
	log.Debug("handleNewTransaction", "g", g.ID, "uuid", ctx.Value(ctxRequestKey))
	currState, err := g.GetCurrentState(msg.ObjectID)
	if err != nil {
		return nil, fmt.Errorf("error getting current state: %v", err)
	}

	trans := msg.toUnsignedStoredTransaction()
	newTip, isAccepted, err := g.StateHandler(ctx, StateTransaction{
		Group:         g.Group,
		Round:         trans.Round,
		ObjectID:      trans.ObjectID,
		Transaction:   trans.Transaction,
		TransactionID: trans.ID(),
		State:         currState.Tip,
	})
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
	sw := safewrap.SafeWrap{}
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

	sw := &safewrap.SafeWrap{}
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
