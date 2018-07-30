package gossip2

import (
	"context"
	"time"

	"crypto/ecdsa"

	"sync"

	"fmt"

	"bytes"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-cid"
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

const MessageType_Proposal = "PROPOSAL"
const MessageType_Prepare = "PREPARE"
const MessageType_TentativeCommit = "TENTATIVE_COMMIT"

var currentStateBucket = []byte("CurrentState")
var transactionBucket = []byte("Transactions")

const DefaultRoundLength = 10 // round length in seconds
const DefaultPhase1Length = 5 // round length in seconds
const DefaultFanout = 3

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
	ObjectId         ObjectId
	Transaction      Transaction
	PreviousTip      []byte
	NewTip           []byte
	Round            int64
	rumorGossipCount int64
}

func (st *storedTransaction) Id() TransactionId {
	return crypto.Keccak256(st.Transaction)
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
	Proposals []storedTransaction
}

type StateHandler func(ctx context.Context, group *consensus.Group, objectId, transaction, currentState []byte) (nextState []byte, accepted bool, err error)
type AcceptedHandler func(ctx context.Context, group *consensus.Group, objectId, transaction, newState []byte) (err error)
type RejectedHandler func(ctx context.Context, group *consensus.Group, objectId, transaction, currentState []byte) (err error)

type handler interface {
	DoRequest(dst *ecdsa.PublicKey, req *network.Request) (chan *network.Response, error)
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

	g.MessageHandler.AssignHandler(MessageType_Proposal, g.Propose)
	if g.Id == "" {
		g.Id = consensus.BlsVerKeyToAddress(g.SignKey.MustVerKey().Bytes()).String()
	}
	if g.RoundLength == 0 {
		g.RoundLength = DefaultRoundLength
	}
	if g.Fanout == 0 {
		g.Fanout = DefaultFanout // good for approx 1k - 9k nodes
	}
	if g.Phase1Length == 0 {
		g.Phase1Length = DefaultPhase1Length
	}
}

func conflictSetId(objectId ObjectId, previousTip *cid.Cid) []byte {
	return append(objectId, previousTip.Bytes()...)
}

// return the round number (rounds are set to 10 seconds)
func (g *Gossiper) roundAt(t time.Time) int64 {
	return t.UTC().Unix() / int64(g.RoundLength)
}

func (g *Gossiper) Propose(ctx context.Context, req network.Request, respChan network.ResponseChan) error {
	msg := &ProposalMessage{}
	err := cbornode.DecodeInto(req.Payload, msg)
	if err != nil {
		return fmt.Errorf("error decoding: %v", err)
	}

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

func (g *Gossiper) HandlePrepare(ctx context.Context, req network.Request, respChan network.ResponseChan) error {

	// save new signatures to the current state
	// if 2/3 have approved
	// gossip the TC message
	// else gossip the combined signature Prepare message

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

func (g *Gossiper) saveTransaction(st *storedTransaction) error {
	sw := dag.SafeWrap{}
	cborNode := sw.WrapObject(st)
	if sw.Err != nil {
		return fmt.Errorf("error wrapping: %v", sw.Err)
	}

	return g.Storage.Set(transactionBucket, st.Id(), cborNode.RawData())
}
