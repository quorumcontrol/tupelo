package gossip2

import (
	"context"
	"time"

	"crypto/ecdsa"

	"sync"

	"fmt"

	"bytes"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/storage"
)

func init() {
	cbornode.RegisterCborType(storedTransaction{})
	cbornode.RegisterCborType(ProposalMessage{})
}

const messageTypeProposal = "PROPOSAL"
const messageTypePrepare = "PREPARE"
const messageTypeTentativeCommit = "TENTATIVE_COMMIT"

var currentStateBucket = []byte("CurrentState")
var transactionBucket = []byte("Transactions")
var conflicSetBuckert = []byte("ConflicSets")

const defaultRoundLength = 10 // round length in seconds
const defaultPhase1Length = 5 // round length in seconds
const defaultFanout = 3

const phasePrepare = "prep"
const phaseTentativeCommit = "tc"
const phaseCommit = "commit"

type ObjectId []byte
type Transaction []byte
type TransactionId []byte
type ConflictSetId []byte
type waitChanHolder map[string]chan bool

type StoredCurrentState struct {
	ObjectId  ObjectId
	Tip       []byte
	Signature consensus.Signature
	Round     int64
}

type storedTransaction struct {
	ObjectId    ObjectId
	Transaction Transaction
	PreviousTip []byte
	NewTip      []byte
	Round       int64
}

func (st *storedTransaction) Id() TransactionId {
	return transactionToID(st.Transaction)
}

type storedRound struct {
	TransactionId TransactionId
	Phase         string
	Signatures    *consensus.SignatureMap
}

type ProposalMessage struct {
	Transaction Transaction
	ObjectId    ObjectId
	PreviousTip []byte
	Round       int64
}

type conflictSet struct {
	Proposals map[string]storedTransaction
	Phase     string
}

type prepareMessage struct {
	transactionID TransactionId
	signatures    consensus.SignatureMap
}

type tentativeCommitMessage struct {
	prepareMessage prepareMessage
	signatures     consensus.SignatureMap
}

type StateHandler func(ctx context.Context, group *consensus.Group, objectId, transaction, currentState []byte) (nextState []byte, accepted bool, err error)
type AcceptedHandler func(ctx context.Context, group *consensus.Group, objectId, transaction, newState []byte) (err error)
type RejectedHandler func(ctx context.Context, group *consensus.Group, objectId, transaction, currentState []byte) (err error)

func transactionToID(trans Transaction) TransactionId {
	return crypto.Keccak256(trans)
}

type handler interface {
	DoRequest(dst *ecdsa.PublicKey, req *network.Request) (chan *network.Response, error)
	Push(dst *ecdsa.PublicKey, req *network.Request) error
	AssignHandler(requestType string, handlerFunc network.HandlerFunc) error
	Start()
	Stop()
}

type Gossiper struct {
	Id              string
	SignKey         *bls.SignKey
	Group           *consensus.Group
	Fanout          int
	RoundLength     int
	Phase1Length    int
	Storage         storage.Storage
	MessageHandler  handler
	StateHandler    StateHandler
	AcceptedHandler AcceptedHandler
	RejectedHandler RejectedHandler
	waits           waitChanHolder
	lock            *sync.RWMutex
}

func (g *Gossiper) Initialize() {
	g.lock = &sync.RWMutex{}
	g.Storage.CreateBucketIfNotExists(currentStateBucket)
	g.Storage.CreateBucketIfNotExists(transactionBucket)
	g.waits = make(waitChanHolder)

	g.MessageHandler.AssignHandler(messageTypeProposal, g.Propose)
	if g.Id == "" {
		g.Id = consensus.BlsVerKeyToAddress(g.SignKey.MustVerKey().Bytes()).String()
	}
	if g.RoundLength == 0 {
		g.RoundLength = defaultRoundLength
	}
	if g.Fanout == 0 {
		g.Fanout = defaultFanout // good for approx 1k - 9k nodes
	}
	if g.Phase1Length == 0 {
		g.Phase1Length = defaultPhase1Length
	}
}

func conflictSetId(objectId ObjectId, previousTip []byte) []byte {
	return append(objectId, previousTip...)
}

// return the round number (rounds are set to 10 seconds)
func (g *Gossiper) roundAt(t time.Time) int64 {
	return t.UTC().Unix() / int64(g.RoundLength)
}

// Propose handles the proposal phase of Gosig. The "leader" in this situation is the owner of a chaintree
func (g *Gossiper) Propose(ctx context.Context, req network.Request, respChan network.ResponseChan) error {
	msg := &ProposalMessage{}
	err := cbornode.DecodeInto(req.Payload, msg)
	if err != nil {
		return fmt.Errorf("error decoding: %v", err)
	}

	err = g.saveNewTransaction(ctx, msg)

	go func() {
		time.Sleep(time.Duration(g.Phase1Length) * time.Second)
		// then prepare it
	}()

	//gossip the msg - maybe as a group of messages?

	resp, err := network.BuildResponse(req.Id, 200, true)
	if err != nil {
		return fmt.Errorf("error building response: %v", err)
	}
	respChan <- resp

	return err
}

func (g *Gossiper) handleProposeGossip(ctx context.Context, req network.Request, respChan network.ResponseChan) error {
	msg := &ProposalMessage{}
	err := cbornode.DecodeInto(req.Payload, msg)
	if err != nil {
		return fmt.Errorf("error decoding: %v", err)
	}

	transBytes, err := g.transBytes(ctx, transactionToID(msg.Transaction))
	if err != nil {
		return fmt.Errorf("error getting transBytes: %v", err)
	}

	if len(transBytes) == 0 {
		err = g.saveNewTransaction(ctx, msg)
		if err != nil {
			return fmt.Errorf("error saving proposal: %v", err)
		}
	}
	// we have checked the transaction so we can gossip it out

	gossipReq, err := network.BuildRequest(messageTypeProposal, msg)
	if err != nil {
		return fmt.Errorf("error building request: %v", err)
	}

	//TODO: terminate gossip in a smarter way
	if msg.Round <= g.roundAt(time.Now())+2 {
		err = g.fanoutRequest(ctx, gossipReq)
		if err != nil {
			return fmt.Errorf("error fanning out: %v", err)
		}
	}

	respChan <- nil
	return nil
}

func (g *Gossiper) transBytes(ctx context.Context, id TransactionId) ([]byte, error) {
	return g.Storage.Get(transactionBucket, id)
}

func (g *Gossiper) saveNewTransaction(ctx context.Context, msg *ProposalMessage) error {

	currRound := g.roundAt(time.Now())

	if msg.Round == 0 {
		msg.Round = currRound
	}

	if msg.Round > currRound {
		return fmt.Errorf("error, round is in the future: %d", msg.Round)
	}

	currState, err := g.GetCurrenState(ctx, msg.ObjectId)
	if err != nil {
		return fmt.Errorf("error getting current state: %v", err)
	}

	if !bytes.Equal(msg.PreviousTip, currState.Tip) {
		return fmt.Errorf("invalid previous tip: %s, currently: %s", msg.PreviousTip, currState.Tip)
	}

	handlerState, isAccepted, err := g.StateHandler(ctx, g.Group, msg.ObjectId, msg.Transaction, currState.Tip)
	if err != nil {
		return fmt.Errorf("error calling state handler: %v", err)
	}

	if !isAccepted {
		return fmt.Errorf("not accepted")
	}

	toStore := &storedTransaction{
		ObjectId:    msg.ObjectId,
		Transaction: msg.Transaction,
		PreviousTip: msg.PreviousTip,
		NewTip:      handlerState,
		Round:       msg.Round,
	}

	err = g.saveTransaction(toStore)
	if err != nil {
		return fmt.Errorf("error saving transaction")
	}

	csID := conflictSetId(msg.ObjectId, msg.PreviousTip)

	cs, err := g.getOrCreateConflictSet(csID)
	if err != nil {
		return fmt.Errorf("error getting conflic set: %v", err)
	}

	cs.Proposals[string(toStore.Id())] = *toStore
	g.saveConflicSet(csID, cs)

	return nil
}

func (g *Gossiper) getOrCreateConflictSet(id ConflictSetId) (cs *conflictSet, err error) {
	g.lock.RLock()
	defer g.lock.RUnlock()
	existingConflicSetBytes, err := g.Storage.Get(conflicSetBuckert, id)
	if err != nil {
		return nil, fmt.Errorf("error getting bytes: %v", err)
	}
	if len(existingConflicSetBytes) == 0 {
		cs = &conflictSet{
			Proposals: make(map[string]storedTransaction),
		}
	} else {
		g.lock.RUnlock()
		err = cbornode.DecodeInto(existingConflicSetBytes, cs)
		if err != nil {
			return nil, fmt.Errorf("error decoding: %v", err)
		}
	}
	return
}

func (g *Gossiper) saveConflicSet(id ConflictSetId, cs *conflictSet) error {
	sw := &dag.SafeWrap{}
	node := sw.WrapObject(cs)
	if sw.Err != nil {
		return fmt.Errorf("error wrapping: %v", sw.Err)
	}
	g.lock.Lock()
	defer g.lock.Unlock()
	return g.Storage.Set(conflicSetBuckert, id, node.RawData())
}

func (g *Gossiper) doPreparePhase(csID ConflictSetId) error {
	// for now just take the first proposal
	cs, err := g.getOrCreateConflictSet(csID)
	if err != nil {
		return fmt.Errorf("error geting conflict set: %v", err)
	}

	trans := g.pickBestProposal(cs)
	cs.Phase = phasePrepare

	//sign this prepare message and send out the prepare

}

func (g *Gossiper) signTransaction(trans Transaction, round int64) consensus.Signature {
	// sign and return the trans
}

func (g *Gossiper) pickBestProposal(cs *conflictSet) *storedTransaction {
	for id, trans := range cs.Proposals {
		return &trans
	}
	return nil
}

// HandlePrepare saves new signatures to the current state
// only do this if we are already in the prepare phase ourselves
// if 2/3 have approved
// gossip the TC message
// else gossip the combined signature Prepare message
func (g *Gossiper) HandlePrepare(ctx context.Context, req network.Request, respChan network.ResponseChan) error {

	return nil
}

// HandleTentativeCommit verifies that 2/3 of the signers have sent out 2/3 of the prepare messages
// if 2/3 have signed the tentative commit then we can commit it
func (g *Gossiper) HandleTentativeCommit(ctx context.Context, req network.Request, respChan network.ResponseChan) error {

	return nil
}

func (g *Gossiper) GetCurrenState(ctx context.Context, objectId ObjectId) (state StoredCurrentState, err error) {
	g.lock.RLock()
	defer g.lock.RUnlock()

	storedBytes, err := g.Storage.Get(currentStateBucket, objectId)
	if err != nil {
		return state, fmt.Errorf("error getting state: %v", err)
	}
	if len(storedBytes) == 0 {
		return state, nil
	}

	err = cbornode.DecodeInto(storedBytes, state)
	if err != nil {
		return state, fmt.Errorf("error decoding: %v", err)
	}

	return state, err
}

func (g *Gossiper) fanoutRequest(_ context.Context, req *network.Request) error {
	for i := 0; i < g.Fanout; i++ {
		rn := g.Group.RandomMember()
		err := g.MessageHandler.Push(rn.DstKey.ToEcdsaPub(), req)
		if err != nil {
			return fmt.Errorf("error pushing: %v", err)
		}
	}
	return nil
}

func (g *Gossiper) saveTransaction(st *storedTransaction) error {
	sw := dag.SafeWrap{}
	cborNode := sw.WrapObject(st)
	if sw.Err != nil {
		return fmt.Errorf("error wrapping: %v", sw.Err)
	}

	return g.Storage.Set(transactionBucket, st.Id(), cborNode.RawData())
}
