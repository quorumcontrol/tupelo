package actors

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo-go-client/client"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/remote"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/types"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
)

const committedKind = "committed"
const ErrBadTransaction = 1

// TupeloNode is the main logic of the entire system,
// consisting of multiple gossipers
type TupeloNode struct {
	middleware.LogAwareHolder

	self              *types.Signer
	notaryGroup       *types.NotaryGroup
	committedGossiper *actor.PID
	conflictSetRouter *actor.PID
	committedStore    *actor.PID
	validatorPool     *actor.PID
	cfg               *TupeloConfig
}

type TupeloConfig struct {
	Self              *types.Signer
	NotaryGroup       *types.NotaryGroup
	CommitStore       storage.Storage
	CurrentStateStore storage.Storage
	PubSubSystem      remote.PubSub
}

func NewTupeloNodeProps(cfg *TupeloConfig) *actor.Props {
	return actor.PropsFromProducer(func() actor.Actor {
		return &TupeloNode{
			self:        cfg.Self,
			notaryGroup: cfg.NotaryGroup,
			cfg:         cfg,
		}
	}).WithReceiverMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (tn *TupeloNode) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		tn.handleStarted(context)
	case *extmsgs.GetTip:
		tn.handleGetTip(context, msg)
	case *messages.GetSyncer:
		tn.handleGetSyncer(context, msg)
	case *messages.StartGossip:
		tn.handleStartGossip(context, msg)
	case *messages.CurrentStateWrapper:
		tn.handleNewCurrentState(context, msg)
	case *extmsgs.Signature:
		context.Forward(tn.conflictSetRouter)
	case *extmsgs.Transaction:
		tn.handleNewTransaction(context)
	case *messages.ValidateTransaction:
		tn.handleNewTransaction(context)
	case *messages.TransactionWrapper:
		tn.handleNewTransaction(context)
	}
}

func (tn *TupeloNode) handleNewCurrentState(context actor.Context, msg *messages.CurrentStateWrapper) {
	if msg.Verified {
		context.Send(tn.committedStore, &extmsgs.Store{Key: msg.CurrentState.CommittedKey(), Value: msg.Value, SkipNotify: msg.Internal})
		err := tn.cfg.CurrentStateStore.Set(msg.CurrentState.CurrentKey(), msg.Value)
		if err != nil {
			panic(fmt.Errorf("error setting current state: %v", err))
		}
		// un-snooze waiting conflict sets
		context.Send(tn.conflictSetRouter, &messages.ActivateSnoozingConflictSets{ObjectID: msg.CurrentState.Signature.ObjectID})
		tn.Log.Infow("commit", "tx", msg.CurrentState.Signature.TransactionID, "seen", msg.Metadata["seen"])
		// if we are the ones creating this current state then broadcast
		if tn.isOnRewardsCommittee(msg.CurrentState.Signature.NewTip) {
			tn.Log.Debugw("publishing new current state", "topic", string(msg.CurrentState.Signature.ObjectID))
			if err := tn.cfg.PubSubSystem.Broadcast(string(msg.CurrentState.Signature.ObjectID), msg.CurrentState); err != nil {
				tn.Log.Errorw("error publishing", "err", err)
			}
		}
	} else {
		tn.Log.Debugw("removing bad current state", "key", msg.Key)
		err := tn.cfg.CurrentStateStore.Delete(msg.Key)
		if err != nil {
			panic(fmt.Errorf("error deleting bad current state: %v", err))
		}
	}
}

func (tn *TupeloNode) handleNewTransaction(context actor.Context) {
	switch msg := context.Message().(type) {
	case *extmsgs.Transaction:
		// broadcaster has sent us a fresh transaction
		tn.validateTransaction(context, &messages.ValidateTransaction{
			Transaction: msg,
		})
	case *messages.ValidateTransaction:
		// snoozed transaction has been activated and needs full validation
		tn.validateTransaction(context, msg)
	case *messages.TransactionWrapper:
		// validatorPool has validated or rejected a transaction we sent it above
		if msg.PreFlight || msg.Accepted {
			context.Send(tn.conflictSetRouter, msg)
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
					errSource = string(msg.TransactionID)
				}
				err := tn.cfg.PubSubSystem.Broadcast(string(msg.Transaction.ObjectID), &extmsgs.Error{
					Source: errSource,
					Code:   ErrBadTransaction,
					Memo:   fmt.Sprintf("bad transaction: %v", msg.Metadata["error"]),
				})
				if err != nil {
					tn.Log.Errorw("error publishing", "err", err)
				}
			}
			msg.StopTrace()
		}
	}
}

func (tn *TupeloNode) validateTransaction(context actor.Context, msg *messages.ValidateTransaction) {
	tn.Log.Debugw("validating transaction", "msg", msg)
	context.Request(tn.validatorPool, &validationRequest{
		transaction: msg.Transaction,
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
		context.Send(tn.conflictSetRouter, &commitNotification{
			store:    msg,
			objectID: currState.Signature.ObjectID,
			height:   currState.Signature.Height,
		})
	}
}

func (tn *TupeloNode) handleStartGossip(context actor.Context, msg *messages.StartGossip) {
	newMsg := &messages.StartGossip{
		System: tn.notaryGroup,
	}
	context.Send(tn.committedGossiper, newMsg)
}

func (tn *TupeloNode) handleGetTip(context actor.Context, msg *extmsgs.GetTip) {
	tn.Log.Debugw("handleGetTip", "tip", msg.ObjectID)
	currStateBits, err := tn.cfg.CurrentStateStore.Get(msg.ObjectID)
	if err != nil {
		panic(fmt.Errorf("error getting tip: %v", err))
	}

	var currState extmsgs.CurrentState

	if len(currStateBits) > 0 {
		_, err = currState.UnmarshalMsg(currStateBits)
		if err != nil {
			panic(fmt.Errorf("error unmarshaling CurrentState: %v", err))
		}
	}

	context.Respond(&currState)
}

func (tn *TupeloNode) handleGetSyncer(context actor.Context, msg *messages.GetSyncer) {
	switch msg.Kind {
	case committedKind:
		context.Forward(tn.committedGossiper)
	default:
		panic("unknown gossiper")
	}
}

func (tn *TupeloNode) handleStarted(context actor.Context) {
	_, err := context.SpawnNamed(tn.cfg.PubSubSystem.NewSubscriberProps(client.TransactionBroadcastTopic), "broadcast-subscriber")
	if err != nil {
		panic(fmt.Sprintf("err spawning broadcast receiver: %v", err))
	}

	committedStore, err := context.SpawnNamed(NewStorageProps(tn.cfg.CommitStore), "committedstore")
	if err != nil {
		panic(fmt.Sprintf("err: %v", err))
	}

	commitSubscriber, err := context.SpawnNamed(actor.PropsFromFunc(tn.handleNewCommit), "commitSubscriber")
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}

	context.Send(committedStore, &messages.Subscribe{Subscriber: commitSubscriber})

	committedProps := NewPushSyncerProps(committedKind, committedStore)
	committedGossiper, err := context.SpawnNamed(NewGossiperProps(committedKind, committedStore, tn.notaryGroup, committedProps), committedKind)
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

	tvConfig := &TransactionValidatorConfig{
		NotaryGroup:       tn.notaryGroup,
		SignatureChecker:  sigChecker,
		CurrentStateStore: tn.cfg.CurrentStateStore,
	}
	validatorPool, err := context.SpawnNamed(NewTransactionValidatorProps(tvConfig), "validator")
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}

	csrConfig := &ConflictSetRouterConfig{
		NotaryGroup:        tn.notaryGroup,
		Signer:             tn.self,
		SignatureGenerator: sigGenerator,
		SignatureChecker:   sigChecker,
		SignatureSender:    sender,
		CurrentStateStore:  tn.cfg.CurrentStateStore,
	}
	router, err := context.SpawnNamed(NewConflictSetRouterProps(csrConfig), "conflictSetRouter")
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}

	tn.conflictSetRouter = router
	tn.committedGossiper = committedGossiper
	tn.committedStore = committedStore
	tn.validatorPool = validatorPool
}

func (tn *TupeloNode) isOnRewardsCommittee(tip []byte) bool {
	// send in a blank signer so it includes the signer
	committee, err := tn.notaryGroup.RewardsCommittee(tip, &types.Signer{})
	if err != nil {
		panic(fmt.Errorf("error getting rewards committee: %v", err))
	}
	for _, signer := range committee {
		if signer.ID == tn.self.ID {
			return true
		}
	}
	return false
}
