package actors

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/storage"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/types"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
)

const mempoolKind = "mempool"
const committedKind = "committed"
const ErrBadTransaction = 1

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
	case *extmsgs.GetTip:
		tn.handleGetTip(context, msg)
	case *messages.GetSyncer:
		tn.handleGetSyncer(context, msg)
	case *messages.StartGossip:
		tn.handleStartGossip(context, msg)
	case *messages.CurrentStateWrapper:
		tn.handleNewCurrentState(context, msg)
	case *extmsgs.Store:
		context.Forward(tn.mempoolStore)
	case *extmsgs.Signature:
		context.Forward(tn.conflictSetRouter)
	case *extmsgs.TipSubscription:
		context.Forward(tn.subscriptionHandler)
	case *messages.ValidateTransaction:
		tn.handleNewTransaction(context)
	case *messages.TransactionWrapper:
		tn.handleNewTransaction(context)
	}
}

func (tn *TupeloNode) handleNewCurrentState(context actor.Context, msg *messages.CurrentStateWrapper) {
	if msg.Verified {
		tn.committedStore.Tell(&extmsgs.Store{Key: msg.CurrentState.CommittedKey(), Value: msg.Value, SkipNotify: msg.Internal})
		err := tn.cfg.CurrentStateStore.Set(msg.CurrentState.CurrentKey(), msg.Value)
		if err != nil {
			panic(fmt.Errorf("error setting current state: %v", err))
		}
		// un-snooze waiting transactions
		tn.conflictSetRouter.Tell(&messages.ProcessSnoozedTransactions{ObjectID: msg.CurrentState.Signature.ObjectID})
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
	case *extmsgs.Store:
		// mempoolStore is notifying us that it just stored a new transaction
		tn.validateTransaction(context, &messages.ValidateTransaction{
			Key:   msg.Key,
			Value: msg.Value,
		})
	case *messages.ValidateTransaction:
		// snoozed transaction has been activated and needs full validation
		tn.validateTransaction(context, msg)
	case *messages.TransactionWrapper:
		// validatorPool has validated or rejected a transaction we sent it above
		if msg.PreFlight || msg.Accepted {
			tn.conflictSetRouter.Tell(msg)
		} else {
			if msg.Stale {
				tn.Log.Debugw("ignoring and cleaning up stale transaction", "msg", msg)
			} else {
				tn.Log.Debugw("removing bad transaction", "msg", msg)
				var errSource string
				if msg.Transaction != nil && msg.Transaction.ObjectID != nil {
					// need this to route the error back to the correct subscribers
					errSource = string(msg.Transaction.ObjectID)
				} else {
					// ...but fallback on this rather than generating a nil deref error
					errSource = string(msg.Key)
				}
				tn.subscriptionHandler.Tell(&extmsgs.Error{
					Source: errSource,
					Code:   ErrBadTransaction,
					Memo:   fmt.Sprintf("bad transaction: %v", msg.Metadata["error"]),
				})
			}
			tn.mempoolStore.Tell(&messages.Remove{Key: msg.Key})
		}
	}
}

func (tn *TupeloNode) validateTransaction(context actor.Context, msg *messages.ValidateTransaction) {
	tn.Log.Debugw("validating transaction", "msg", msg)
	context.Request(tn.validatorPool, &validationRequest{
		key:   msg.Key,
		value: msg.Value,
	})
}

// this function is its own actor
func (tn *TupeloNode) handleNewCommit(context actor.Context) {
	switch msg := context.Message().(type) {
	case *extmsgs.Store:
		tn.Log.Debugw("new commit")
		var currState extmsgs.CurrentState
		_, err := currState.UnmarshalMsg(msg.Value)
		if err != nil {
			panic(fmt.Errorf("error unmarshaling: %v", err))
		}
		tn.conflictSetRouter.Tell(&commitNotification{
			store:    msg,
			objectID: currState.Signature.ObjectID,
		})
	}
}

func (tn *TupeloNode) handleStartGossip(context actor.Context, msg *messages.StartGossip) {
	newMsg := &messages.StartGossip{
		System: tn.notaryGroup,
	}
	tn.mempoolGossiper.Tell(newMsg)
	tn.committedGossiper.Tell(newMsg)
}

func (tn *TupeloNode) handleGetTip(context actor.Context, msg *extmsgs.GetTip) {
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
		CurrentStateStore:  tn.cfg.CurrentStateStore,
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
