package actors

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/types"
)

const mempoolKind = "mempool"
const committedKind = "committed"

// TupeloNode is the main logic of the entire system,
// consisting of multiple gossipers
type TupeloNode struct {
	middleware.LogAwareHolder

	self               *types.Signer
	notaryGroup        *types.NotaryGroup
	mempoolGossiper    *actor.PID
	committedGossiper  *actor.PID
	currentStateStore  *actor.PID
	transactionHandler *actor.PID
	currentRound       uint64
}

func NewTupeloNodeProps(self *types.Signer, ng *types.NotaryGroup) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &TupeloNode{
			self:        self,
			notaryGroup: ng,
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (tn *TupeloNode) Receive(context actor.Context) {
	// defer func() {
	// 	if r := recover(); r != nil {
	// 		tn.Log.Errorw("error", "r", r)
	// 		debug.PrintStack()
	// 	}
	// }()

	switch msg := context.Message().(type) {
	case *actor.Started:
		tn.handleStarted(context)
	case *messages.GetSyncer:
		tn.handleGetSyncer(context, msg)
	case *messages.StartGossip:
		tn.handleStartGossip(context, msg)
	case *messages.RoundTransition:
		tn.handleRoundTransition(context, msg)
	case *messages.NewValidatedTransaction:
		tn.handleNewValidatedTransaction(context, msg)
	case *messages.NewValidCurrentState:
		tn.handleNewValidCurrentState(context, msg)
	case *messages.GetTip:
		tn.handleGetTip(context, msg)
	case *messages.Store:
		context.Forward(tn.mempoolGossiper)
	case *messages.Signature:
		context.Forward(tn.transactionHandler)
	case *messages.CurrentState:
		context.Forward(tn.committedGossiper)
	case *messages.Get:
		context.Forward(tn.mempoolGossiper)
	case *messages.MemPoolCleanup:
		tn.mempoolGossiper.Tell(&messages.BulkRemove{ObjectIDs: msg.Transactions})
	}
}

func (tn *TupeloNode) handleGetSyncer(context actor.Context, msg *messages.GetSyncer) {
	switch msg.Kind {
	case mempoolKind:
		context.Forward(tn.mempoolGossiper)
	case committedKind:
		context.Forward(tn.committedGossiper)
	default:
		panic("unknown gossiper")
	}
}

func (tn *TupeloNode) handleStartGossip(context actor.Context, msg *messages.StartGossip) {
	newMsg := &messages.StartGossip{
		System: tn.notaryGroup,
	}
	tn.mempoolGossiper.Tell(newMsg)
	tn.committedGossiper.Tell(newMsg)
}

func (tn *TupeloNode) handleStarted(context actor.Context) {
	currentStateStore, err := context.SpawnNamed(NewStorageProps(), "currentStateStore")
	if err != nil {
		panic(fmt.Sprintf("err: %v", err))
	}

	mempoolValidator, err := context.SpawnNamed(NewMemPoolValidatorProps(currentStateStore), "mempoolvalidator")
	if err != nil {
		panic(fmt.Sprintf("err: %v", err))
	}
	// TODO: this should be a different validator
	committedValidator, err := context.SpawnNamed(NewCommitValidatorProps(currentStateStore, tn.notaryGroup), "committedvalidator")
	if err != nil {
		panic(fmt.Sprintf("err: %v", err))
	}

	mempoolGossiper, err := context.SpawnNamed(NewGossiperProps(mempoolKind, mempoolValidator, tn.notaryGroup), mempoolKind)
	if err != nil {
		panic(fmt.Sprintf("error spawning mempool: %v", err))
	}
	committedGossiper, err := context.SpawnNamed(NewGossiperProps(committedKind, committedValidator, tn.notaryGroup), committedKind)
	if err != nil {
		panic(fmt.Sprintf("error spawning mempool: %v", err))
	}

	transactionHandler, err := context.SpawnNamed(NewValidTransactionHandlerProps(currentStateStore, tn.self, tn.notaryGroup), "transactionValidator")
	if err != nil {
		panic(fmt.Sprintf("error spawning mempool: %v", err))
	}

	tn.mempoolGossiper = mempoolGossiper
	tn.committedGossiper = committedGossiper
	tn.transactionHandler = transactionHandler
	tn.currentStateStore = currentStateStore
}

func (tn *TupeloNode) handleNewValidatedTransaction(context actor.Context, msg *messages.NewValidatedTransaction) {

	context.Request(tn.transactionHandler, msg)
}

func (tn *TupeloNode) handleNewValidCurrentState(context actor.Context, msg *messages.NewValidCurrentState) {
	tn.Log.Infow("committing", "cs", msg.CurrentState.StorageKey(), "curr", msg.CurrentState.CurrentKey())
	tn.currentStateStore.Tell(&messages.Store{Key: msg.CurrentState.CurrentKey(), Value: msg.Value})
	context.Request(tn.transactionHandler, msg.CurrentState)
}

func (tn *TupeloNode) handleGetTip(context actor.Context, msg *messages.GetTip) {
	tn.currentStateStore.Request(&messages.Get{Key: msg.ObjectID}, context.Sender())
}

func (tn *TupeloNode) handleRoundTransition(context actor.Context, msg *messages.RoundTransition) {
	if msg.NextRound != tn.currentRound+1 {
		panic(fmt.Sprintf("invalid round: %d, current is: %d", msg.NextRound, tn.currentRound))
	}

	// verifier for the round pool keeps track of IBFs
	// once it has seen 2/3 of the round IBFs, it will trigger a round ready message

	// snapshot the mempool
	// write our IBF to the round pool
	// wait for the verifier to notify us

	// write block to the round pool
	// write signature to the round pool
	// ... normal rules for 2/3 and/or deadlock ...
	// move block into the committed pool

	// ----- this is theory to implement current but with an eye to blocks ----
	// need "currentTip" API for the mempool (this is needed anyway)
	// move all validated mempool into the round pool
	// validator for the round pool will exclude trans not in the mempool (this would get trashed in the real thing)
	//		- but accept a new type: DONE
	// validator will do the send signature
	// new signature handler (I believe we still want this)
	// sig handler puts the DONE message into the round pool
}
