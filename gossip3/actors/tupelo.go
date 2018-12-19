package actors

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/eventstream"
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

	self              *types.Signer
	notaryGroup       *types.NotaryGroup
	mempoolGossiper   *actor.PID
	committedGossiper *actor.PID

	conflictSetRouter *actor.PID

	currentStateStore *actor.PID
	mempoolStore      *actor.PID
	committedStore    *actor.PID

	validatorPool *actor.PID
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
	switch msg := context.Message().(type) {
	case *actor.Started:
		tn.handleStarted(context)
	case *messages.Get:
		context.Forward(tn.mempoolStore)
	case *messages.GetTip:
		tn.handleGetTip(context, msg)
	case *messages.GetSyncer:
		tn.handleGetSyncer(context, msg)
	case *messages.StartGossip:
		tn.handleStartGossip(context, msg)
	case *messages.CurrentStateWrapper:
		tn.handleNewCurrentState(context, msg)
	case *messages.Store:
		context.Forward(tn.mempoolStore)
	case *messages.Signature:
		context.Forward(tn.conflictSetRouter)
	}
}

func (tn *TupeloNode) handleNewCurrentState(context actor.Context, msg *messages.CurrentStateWrapper) {
	if msg.Verified {
		eventstream.Publish(msg)
		tn.currentStateStore.Tell(&messages.Store{Key: msg.CurrentState.CurrentKey(), Value: msg.Value})
	}
}

// this function is its own actor
func (tn *TupeloNode) handleNewTransaction(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.Store:
		tn.Log.Debugw("new transaction", "msg", msg)
		context.Request(tn.validatorPool, msg)
	case *messages.TransactionWrapper:
		if msg.Accepted {
			tn.conflictSetRouter.Tell(msg)
		} else {
			tn.Log.Debugw("removing bad transaction", "msg", msg)
			tn.mempoolStore.Tell(&messages.Remove{Key: msg.Key})
		}
	}
}

// this function is its own actor
func (tn *TupeloNode) handleNewCommit(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.Store:
		tn.Log.Debugw("new commit", "msg", msg)
		context.Request(tn.validatorPool, msg)
	case *messages.CurrentStateWrapper:
		if msg.Verified {
			tn.committedStore.Tell(&messages.Store{Key: msg.Key, Value: msg.Value})
			// TODO: fill in the below with the cleanup messages
			// tn.mempoolStore.Tell(&messages.BulkRemove{ObjectIDs: nil})
		} else {
			//TODO: remove from commit pool
		}
	}
}

func (tn *TupeloNode) handleStartGossip(context actor.Context, msg *messages.StartGossip) {
	newMsg := &messages.StartGossip{
		System: tn.notaryGroup,
	}
	tn.mempoolGossiper.Tell(newMsg)
	tn.committedGossiper.Tell(newMsg)
}

func (tn *TupeloNode) handleGetTip(context actor.Context, msg *messages.GetTip) {
	tn.currentStateStore.Request(&messages.Get{Key: msg.ObjectID}, context.Sender())
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

func (tn *TupeloNode) handleStarted(context actor.Context) {
	currentStateStore, err := context.SpawnNamed(NewStorageProps(), "currentStateStore")
	if err != nil {
		panic(fmt.Sprintf("err: %v", err))
	}

	mempoolStore, err := context.SpawnNamed(NewStorageProps(), "mempoolvalidator")
	if err != nil {
		panic(fmt.Sprintf("err: %v", err))
	}

	mempoolSubscriber, err := context.SpawnNamed(actor.FromFunc(tn.handleNewTransaction), "mempoolSubscriber")
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}

	mempoolStore.Tell(&messages.Subscribe{Subscriber: mempoolSubscriber})

	committedStore, err := context.SpawnNamed(NewStorageProps(), "committedvalidator")
	if err != nil {
		panic(fmt.Sprintf("err: %v", err))
	}

	commitSubscriber, err := context.SpawnNamed(actor.FromFunc(tn.handleNewCommit), "commitSubscriber")
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}

	committedStore.Tell(&messages.Subscribe{Subscriber: commitSubscriber})

	mempoolPusherProps := NewPushSyncerProps(mempoolKind, mempoolStore)
	committedProps := NewPushSyncerProps(committedKind, committedStore)

	mempoolGossiper, err := context.SpawnNamed(NewGossiperProps(mempoolKind, mempoolStore, tn.notaryGroup, mempoolPusherProps), mempoolKind)
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}
	committedGossiper, err := context.SpawnNamed(NewGossiperProps(committedKind, committedStore, tn.notaryGroup, committedProps), committedKind)
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}

	validatorPool, err := context.SpawnNamed(NewTransactionValidatorProps(currentStateStore), "validator")
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}

	sender, err := context.SpawnNamed(NewSignatureSenderProps(), "signatureSender")
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}

	sigGenerator, err := context.SpawnNamed(NewSignatureGeneratorProps(tn.self, tn.notaryGroup), "signatureGenerator")
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}

	sigChecker, err := context.SpawnNamed(NewSignatureVerifier(), "sigChecker")
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}

	cfg := &ConflictSetRouterConfig{
		NotaryGroup:        tn.notaryGroup,
		Signer:             tn.self,
		SignatureGenerator: sigGenerator,
		SignatureChecker:   sigChecker,
		SignatureSender:    sender,
	}
	router, err := context.SpawnNamed(NewConflictSetRouterProps(cfg), "conflictSetRouter")
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}

	tn.conflictSetRouter = router
	tn.mempoolGossiper = mempoolGossiper
	tn.committedGossiper = committedGossiper
	tn.currentStateStore = currentStateStore
	tn.mempoolStore = mempoolStore
	tn.committedStore = committedStore
	tn.validatorPool = validatorPool
}
