package actors

import (
	"fmt"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/AsynkronIT/protoactor-go/router"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"go.uber.org/zap"
)

const stateHandlerConcurrency = 10

type MemPoolValidator struct {
	middleware.LogAwareHolder
	*Storage

	subscriptions     []*actor.PID
	currentStateStore *actor.PID
	handlerPool       *actor.PID
	isWorking         bool
}

func (mpv *MemPoolValidator) SetLog(log *zap.SugaredLogger) {
	mpv.Log = log
	mpv.Storage.SetLog(log)
}

func NewMemPoolValidatorProps(currentState *actor.PID) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &MemPoolValidator{
			currentStateStore: currentState,
			Storage:           NewInitializedStorageStruct(),
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (mpv *MemPoolValidator) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		mpv.handlerPool = context.Spawn(router.NewRoundRobinPool(stateHandlerConcurrency).WithProducer(func() actor.Actor {
			return &stateHandler{
				currentStateActor: mpv.currentStateStore,
			}
		}).WithMiddleware(
			middleware.LoggingMiddleware,
			plugin.Use(&middleware.LogPlugin{}),
		))
	case *actor.ReceiveTimeout:
		mpv.Log.Debugw("validator clear")
		context.SetReceiveTimeout(0)
		mpv.isWorking = false
		mpv.notifyClear()

	// Override the default storage Store message handling
	// by inserting a validator
	case *messages.Store:
		mpv.handleStore(context, msg)
	case *messages.SubscribeValidatorWorking:
		mpv.subscriptions = append(mpv.subscriptions, msg.Actor)

	// actually do the store now that it's passed a validator
	case *stateTransactionResponse:
		mpv.Log.Debugw("stateTransactionResponse", "msg", msg)
		if msg.accepted {
			didSet, err := mpv.Add(msg.stateTransaction.TransactionID, msg.stateTransaction.payload)
			if err != nil {
				mpv.Log.Errorw("error storing: %v", err)
				return
			}
			if didSet {
				context.Parent().Tell(&messages.NewValidatedTransaction{
					ObjectID:      msg.stateTransaction.ObjectID,
					ConflictSetID: msg.stateTransaction.ConflictSetID,
					TransactionID: msg.stateTransaction.TransactionID,
					NewTip:        msg.nextState,
					OldTip:        msg.stateTransaction.CurrentState,
				})
			}
		} else {
			mpv.Log.Debugw("rejected transaction", "id", msg.stateTransaction.TransactionID, "err", msg.err)
		}
	default:
		// handle the standard GET/GetPrefix, etc
		mpv.Storage.Receive(context)
	}
}

func (mpv *MemPoolValidator) handleStore(context actor.Context, msg *messages.Store) {
	mpv.Log.Debugw("store", "key", msg.Key, "lenVal", len(msg.Value), "sender", context.Sender())
	if !mpv.isWorking {
		// only notify on start working
		mpv.notifyWorking()
		mpv.isWorking = true
		context.SetReceiveTimeout(10 * time.Millisecond)
	}

	alreadyExists, err := mpv.storage.Exists(msg.Key)
	if err != nil {
		panic(fmt.Sprintf("error checking existance: %v", err))
	}
	if !alreadyExists {
		mpv.handlerPool.Request(msg, context.Self())
	}
}

func (mpv *MemPoolValidator) notifyWorking() {
	for _, act := range mpv.subscriptions {
		act.Tell(&messages.ValidatorWorking{})
	}
}

func (mpv *MemPoolValidator) notifyClear() {
	for _, act := range mpv.subscriptions {
		act.Tell(&messages.ValidatorClear{})
	}
}
