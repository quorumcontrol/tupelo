package actors

import (
	"fmt"

	"github.com/quorumcontrol/tupelo-go-sdk/consensus"

	"github.com/gogo/protobuf/proto"
	datastore "github.com/ipfs/go-datastore"
	"github.com/quorumcontrol/messages/build/go/services"
	"github.com/quorumcontrol/messages/build/go/signatures"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
)

// TupeloNode is the main logic of the entire system,
// consisting of multiple gossipers
type TupeloNode struct {
	middleware.LogAwareHolder

	self                      *types.Signer
	notaryGroup               *types.NotaryGroup
	conflictSetRouter         *actor.PID
	validatorPool             *actor.PID
	signatureChecker          *actor.PID
	currentStateExchangeActor *actor.PID
	cfg                       *TupeloConfig
}

type TupeloConfig struct {
	Self              *types.Signer
	NotaryGroup       *types.NotaryGroup
	CurrentStateStore datastore.Batching
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
	case *services.GetTipRequest:
		tn.handleGetTip(context, msg)
	case *messages.CurrentStateWrapper:
		tn.handleNewCurrentStateWrapper(context, msg)
	case *signatures.Signature:
		context.Forward(tn.conflictSetRouter)
	case *services.AddBlockRequest:
		tn.handleNewTransaction(context)
	case *messages.ValidateTransaction:
		tn.handleNewTransaction(context)
	case *messages.TransactionWrapper:
		tn.handleNewTransaction(context)
	case *services.RequestCurrentStateSnapshot:
		context.Forward(tn.currentStateExchangeActor)
	}
}

func (tn *TupeloNode) handleNewCurrentStateWrapper(context actor.Context, msg *messages.CurrentStateWrapper) {
	if msg.Verified {
		tn.Log.Infow("commit", "tx", msg.CurrentState.TransactionId, "seen", msg.Metadata["seen"])
		err := tn.cfg.CurrentStateStore.Put(datastore.NewKey(string(msg.CurrentState.ObjectId)), msg.MustMarshal())
		if err != nil {
			panic(fmt.Errorf("error setting current state: %v", err))
		}
		tn.Log.Debugw("tupelo node sending activatesnoozingconflictsets", "ObjectId", msg.CurrentState.ObjectId)
		// un-snooze waiting conflict sets
		context.Send(tn.conflictSetRouter, &messages.ActivateSnoozingConflictSets{ObjectId: msg.CurrentState.ObjectId})

		// if we are the ones creating this current state then broadcast
		if msg.Internal {
			tn.Log.Debugw("publishing new current state", "topic", string(msg.CurrentState.ObjectId))
			if err := tn.cfg.PubSubSystem.Broadcast(string(msg.CurrentState.ObjectId), msg.CurrentState); err != nil {
				tn.Log.Errorw("error publishing", "err", err)
			}
		}
	}
}

// this function is its own actor
func (tn *TupeloNode) handleNewTransaction(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		_, err := context.SpawnNamed(tn.cfg.PubSubSystem.NewSubscriberProps(tn.notaryGroup.Config().TransactionTopic), "broadcast-subscriber")
		if err != nil {
			panic(fmt.Sprintf("error spawning broadcast receiver: %v", err))
		}
	case *services.AddBlockRequest:
		// broadcaster has sent us a fresh transaction
		tn.Log.Debugw("received block request message")
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
			sp := msg.NewSpan("tupelo-invalid")
			sp.SetTag("error", true)
			sp.Finish()
			msg.StopTrace()
		}
	}
}

func (tn *TupeloNode) validateTransaction(context actor.Context, msg *messages.ValidateTransaction) {
	tn.Log.Debugw("validating transaction", "TransactionId", consensus.RequestID(msg.Transaction))
	context.Request(tn.validatorPool, &validationRequest{
		transaction: msg.Transaction,
	})
}

func (tn *TupeloNode) handleGetTip(context actor.Context, msg *services.GetTipRequest) {
	tn.Log.Debugw("handleGetTip", "chainId", msg.ChainId)
	currStateBits, err := tn.cfg.CurrentStateStore.Get(datastore.NewKey(msg.ChainId))
	if err != nil && err != datastore.ErrNotFound {
		tn.Log.Errorw("error getting tip", "chainId", msg.ChainId, "err", err)
		return
	}

	currState := &signatures.TreeState{}
	if len(currStateBits) > 0 {
		tn.Log.Debugw("could get current state for chain tree", "chainId", msg.ChainId)
		err = proto.Unmarshal(currStateBits, currState)
		if err != nil {
			tn.Log.Errorw("error unmarshaling CurrentState", "err", err)
			return
		}
	} else {
		tn.Log.Debugw("couldn't get current state for chain tree", "chainId", msg.ChainId)
	}

	context.Respond(currState)
}

func (tn *TupeloNode) handleStarted(context actor.Context) {
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
	tn.signatureChecker = sigChecker

	_, err = context.SpawnNamed(actor.PropsFromFunc(tn.handleNewTransaction), "transaction-handler")
	if err != nil {
		panic(fmt.Sprintf("error spawning transaction handler"))
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
	tn.validatorPool = validatorPool

	csrConfig := &ConflictSetRouterConfig{
		NotaryGroup:        tn.notaryGroup,
		Signer:             tn.self,
		SignatureGenerator: sigGenerator,
		SignatureChecker:   sigChecker,
		SignatureSender:    sender,
		CurrentStateStore:  tn.cfg.CurrentStateStore,
		PubSubSystem:       tn.cfg.PubSubSystem,
	}
	router, err := context.SpawnNamed(NewConflictSetRouterProps(csrConfig), "conflictSetRouter")
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}
	tn.conflictSetRouter = router

	currentStateExchangeConfig := &CurrentStateExchangeConfig{
		ConflictSetRouter: router,
		CurrentStateStore: tn.cfg.CurrentStateStore,
	}
	currentStateExchangeActor, err := context.SpawnNamed(NewCurrentStateExchangeProps(currentStateExchangeConfig), "currentStateExchange")
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}
	tn.currentStateExchangeActor = currentStateExchangeActor

	if tn.cfg.NotaryGroup.Size() > 1 {
		var otherSigner *actor.PID

		for otherSigner == nil {
			aSigner := tn.cfg.NotaryGroup.GetRandomSigner()
			if aSigner.ID != tn.self.ID {
				otherSigner = aSigner.Actor
			}
		}

		context.Send(tn.currentStateExchangeActor, &messages.DoCurrentStateExchange{Destination: otherSigner})
	}
}
