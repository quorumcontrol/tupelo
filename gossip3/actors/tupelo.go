package actors

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/types"
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

	mempoolStore        *actor.PID
	committedStore      *actor.PID
	subscriptionHandler *actor.PID

	validatorPool *actor.PID
	cfg           *TupeloConfig
}

type TupeloConfig struct {
	Self              *types.Signer
	NotaryGroup       *types.NotaryGroup
	CommitStore       storage.Storage
	CurrentStateStore storage.Storage
}

func NewTupeloNodeProps(cfg *TupeloConfig) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &TupeloNode{
			self:        cfg.Self,
			notaryGroup: cfg.NotaryGroup,
			cfg:         cfg,
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
	case *messages.TipSubscription:
		context.Forward(tn.subscriptionHandler)
	}
}

func (tn *TupeloNode) handleNewCurrentState(context actor.Context, msg *messages.CurrentStateWrapper) {
	if msg.Verified {
		tn.committedStore.Tell(&messages.Store{Key: msg.CurrentState.CommittedKey(), Value: msg.Value, SkipNotify: msg.Internal})
		err := tn.cfg.CurrentStateStore.Set(msg.CurrentState.CommittedKey(), msg.Value)
		if err != nil {
			panic(fmt.Errorf("error setting current state: %v", err))
		}
		// cleanup the transactions
		ids := make([][]byte, len(msg.CleanupTransactions), len(msg.CleanupTransactions))
		for i, trans := range msg.CleanupTransactions {
			ids[i] = trans.TransactionID
		}
		tn.mempoolStore.Tell(&messages.BulkRemove{ObjectIDs: ids})
		tn.Log.Infow("commit", "tx", msg.CurrentState.Signature.TransactionID, "seen", msg.Metadata["seen"])
		tn.subscriptionHandler.Tell(msg)
	} else {
		tn.Log.Debugw("removing bad current state", "key", msg.Key)
		err := tn.cfg.CurrentStateStore.Delete(msg.Key)
		if err != nil {
			panic(fmt.Errorf("error deleting bad current state: %v", err))
		}
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
		tn.Log.Debugw("new commit")
		tn.conflictSetRouter.Tell(msg)
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
	tip, err := tn.cfg.CurrentStateStore.Get(msg.ObjectID)
	if err != nil {
		panic(fmt.Errorf("error getting tip: %v", err))
	}
	context.Respond(tip)
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
	mempoolStore, err := context.SpawnNamed(NewStorageProps(storage.NewLockFreeMemStorage()), "mempoolvalidator")
	if err != nil {
		panic(fmt.Sprintf("err: %v", err))
	}

	mempoolSubscriber, err := context.SpawnNamed(actor.FromFunc(tn.handleNewTransaction), "mempoolSubscriber")
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}

	mempoolStore.Tell(&messages.Subscribe{Subscriber: mempoolSubscriber})

	committedStore, err := context.SpawnNamed(NewStorageProps(tn.cfg.CommitStore), "committedstore")
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

	validatorPool, err := context.SpawnNamed(NewTransactionValidatorProps(tn.cfg.CurrentStateStore), "validator")
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

	subHandler, err := context.SpawnNamed(NewSubscriptionHandlerProps(), "subscriptionHandler")
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}

	tn.conflictSetRouter = router
	tn.mempoolGossiper = mempoolGossiper
	tn.committedGossiper = committedGossiper
	tn.mempoolStore = mempoolStore
	tn.committedStore = committedStore
	tn.validatorPool = validatorPool
	tn.subscriptionHandler = subHandler
}
