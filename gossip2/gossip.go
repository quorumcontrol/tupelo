package gossip2

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/storage"
)

func init() {
	cbornode.RegisterCborType(gossipMessage{})
}

type objectID []byte
type messageID []byte
type transaction []byte
type transactionID []byte
type conflictSetID []byte
type tip []byte
type phase int8
type roundSignatures map[string]storedTransaction

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
	objectID       objectID
	tip            tip
	round          int64
	proofSignature consensus.Signature
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

type storedTransaction struct {
	objectID    objectID
	previousTip tip
	newTip      tip
	transaction transaction
	round       int64
	signatures  consensus.SignatureMap
}

func (st *storedTransaction) signPrepare(key *bls.SignKey) (*consensus.Signature, error) {
	return consensus.BlsSign(st.msgToSignPrepare(), key)
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
	MessageHandler   handler
	ID               string
	SignKey          *bls.SignKey
	Group            *consensus.Group
	Storage          storage.Storage
	StateHandler     stateHandler
	AcceptedHandler  acceptedHandler
	RejectedHandler  rejectedHandler
	RoundLength      int
	lockLock         *sync.RWMutex
	signaturesInPlay roundSignatures
}

// Initialize sets up the storage and data structures of the gossiper
func (g *Gossiper) Initialize() {

	g.signaturesInPlay = make(roundSignatures)
}

func (g *Gossiper) roundAt(t time.Time) int64 {
	return t.UTC().Unix() / int64(g.RoundLength)
}

func msgToConflictSetID(msg *gossipMessage) conflictSetID {
	return crypto.Keccak256(append(msg.objectID, msg.previousTip...))
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

	return nil
}

func handlePrepareMessage(ctx context.Context, msg *gossipMessage) error {
	if msg.phase != phasePrepare {
		return fmt.Errorf("incorrect phase: %d", msg.phase)
	}

	csId := 
}

func transactionToID(trans transaction, round int64) transactionID {
	return crypto.Keccak256(append(trans, roundToBytes(round)...))
}

func (g *Gossiper) getCurrentState(objID objectID) (currState currentState, err error) {
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

func roundToBytes(round int64) []byte {
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(round))
	return b
}
