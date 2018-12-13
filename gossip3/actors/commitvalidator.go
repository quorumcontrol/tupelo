package actors

import (
	"bytes"
	"fmt"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/AsynkronIT/protoactor-go/router"
	"github.com/quorumcontrol/tupelo/bls"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/types"
	"go.uber.org/zap"
)

const currentStateValidatorMaxConcurrency = 10

type CommitValidator struct {
	middleware.LogAwareHolder
	*Storage

	notaryGroup *types.NotaryGroup

	subscriptions     []*actor.PID
	currentStateStore *actor.PID
	handlerPool       *actor.PID
	isWorking         bool
}

type internalCurrentStateValidated struct {
	accepted     bool
	Key          []byte
	Value        []byte
	CurrentState *messages.CurrentState
}

func (cv *CommitValidator) SetLog(log *zap.SugaredLogger) {
	cv.Log = log
	cv.Storage.SetLog(log)
}

func NewCommitValidatorProps(currentState *actor.PID, notaryGroup *types.NotaryGroup) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &CommitValidator{
			currentStateStore: currentState,
			Storage:           NewInitializedStorageStruct(),
			notaryGroup:       notaryGroup,
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (cv *CommitValidator) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		cv.handlerPool = context.Spawn(
			router.NewRoundRobinPool(currentStateValidatorMaxConcurrency).WithFunc(
				cv.validateCurrentState,
			).WithMiddleware(
				middleware.LoggingMiddleware,
				plugin.Use(&middleware.LogPlugin{}),
			))
	case *actor.ReceiveTimeout:
		cv.Log.Debugw("validator clear")
		context.SetReceiveTimeout(0)
		cv.isWorking = false
		cv.notifyClear()

	case *messages.CurrentState:
		cv.handlerPool.Request(msg, context.Self())

	// Override the default storage Store message handling
	// by inserting a validator
	case *messages.Store:
		if !cv.isWorking {
			// only notify on start working
			cv.notifyWorking()
			cv.isWorking = true
			context.SetReceiveTimeout(10 * time.Millisecond)
		}
		cv.handleStore(context, msg)
	case *messages.SubscribeValidatorWorking:
		cv.subscriptions = append(cv.subscriptions, msg.Actor)

	// actually do the store now that it's passed a validator
	case *internalCurrentStateValidated:
		cv.Log.Debugw("internalCurrentStateValidated", "msg", msg)
		if msg.accepted {
			didSet, err := cv.Add(msg.Key, msg.Value)
			if err != nil {
				cv.Log.Errorw("error storing: %v", err)
				return
			}
			if didSet {
				context.Parent().Tell(&messages.NewValidCurrentState{
					CurrentState: msg.CurrentState,
					Key:          msg.Key,
					Value:        msg.Value,
				})
			}
		} else {
			cv.Log.Infow("rejected transaction", "id", msg.Key)
		}
	default:
		// handle the standard GET/GetPrefix, etc
		cv.Storage.Receive(context)
	}
}

func (cv *CommitValidator) handleStore(context actor.Context, msg *messages.Store) {
	alreadyExists, err := cv.storage.Exists(msg.Key)
	if err != nil {
		panic(fmt.Sprintf("error checking existance: %v", err))
	}
	if !alreadyExists {
		cv.handlerPool.Request(msg, context.Self())
	}
}

func (cv *CommitValidator) notifyWorking() {
	for _, act := range cv.subscriptions {
		act.Tell(&messages.ValidatorWorking{})
	}
}

func (cv *CommitValidator) notifyClear() {
	for _, act := range cv.subscriptions {
		act.Tell(&messages.ValidatorClear{})
	}
}

// this function is itself an actor
func (cv *CommitValidator) validateCurrentState(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.Store:
		var currState messages.CurrentState
		_, err := currState.UnmarshalMsg(msg.Value)
		if err != nil {
			cv.Log.Errorw("error, invalid current state storage", "err", err)
			return
		}
		context.Self().Request(&currState, context.Sender())
	case *messages.CurrentState:
		knownCurrentInt, err := context.RequestFuture(cv.currentStateStore, &messages.Get{Key: msg.CurrentKey()}, 2*time.Second).Result()
		if err != nil {
			panic(fmt.Sprintf("error requesting current state: %v", err))
		}
		bits := knownCurrentInt.([]byte)
		var knownCurrent messages.CurrentState
		if len(bits) > 0 {
			_, err := knownCurrent.UnmarshalMsg(bits)
			if err != nil {
				panic(fmt.Sprintf("error unmarshaling: %v", err))
			}
		}
		if len(knownCurrent.OldTip) != 0 || !bytes.Equal(knownCurrent.OldTip, msg.OldTip) {
			// then we know this is not building on the right thing, maybe it will be true in the future,
			// but we can ignore for now
			context.Respond(&internalCurrentStateValidated{accepted: false, CurrentState: msg})
			return
		}

		didSigners := make([]*types.Signer, 0, len(msg.Signature.Signers))
		for i, b := range msg.Signature.Signers {
			if b {
				didSigners = append(didSigners, cv.notaryGroup.SignerAtIndex(i))
			}
		}
		if len(didSigners) < cv.notaryGroup.QuorumCount() {
			context.Respond(&internalCurrentStateValidated{accepted: false, CurrentState: msg})
			return
		}

		toSign := append(msg.ObjectID, append(msg.OldTip, msg.Tip...)...)

		keys := make([][]byte, len(didSigners), len(didSigners))
		for i, signer := range didSigners {
			keys[i] = signer.VerKey.Bytes()
		}
		didVerify, err := bls.VerifyMultiSig(msg.Signature.Signature, toSign, keys)
		if err != nil {
			panic(fmt.Sprintf("error verifying multisig: %v", err))
		}
		if didVerify {
			context.Respond(&internalCurrentStateValidated{
				accepted:     true,
				CurrentState: msg,
				Key:          msg.StorageKey(),
				Value:        msg.MustBytes(),
			})
			return
		}

		context.Respond(&internalCurrentStateValidated{accepted: false, CurrentState: msg})
	}
}
