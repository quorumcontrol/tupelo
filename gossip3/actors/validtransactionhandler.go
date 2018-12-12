package actors

import (
	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/types"
)

type conflictSet struct {
	ID         string
	done       bool
	signatures signatureMap
}
type conflictSetMap map[string]*conflictSet
type signatureMap map[string]*messages.Signature

type ValidTransactionHandler struct {
	middleware.LogAwareHolder

	self         *types.Signer
	group        *types.NotaryGroup
	currentState *actor.PID
	conflictSets conflictSetMap
}

func NewValidTransactionHandlerProps(currentState *actor.PID) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &ValidTransactionHandler{
			currentState: currentState,
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func newConflictSet(id string) *conflictSet {
	return &conflictSet{
		ID:         id,
		signatures: make(signatureMap),
	}
}

func (vth *ValidTransactionHandler) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.Signature:
		vth.handleNewSignature(context, msg)
	case *messages.NewValidatedTransaction:
		vth.handleNewValidatedTransaction(context, msg)
	}
}

func (vth *ValidTransactionHandler) handleNewValidatedTransaction(context actor.Context, msg *messages.NewValidatedTransaction) {
	cs := vth.getConflictSet(msg.ConflictSetID)
	if cs.done {
		// nothing to do here
		return
	}
	// Sign and send transaction

}

func (vth *ValidTransactionHandler) handleNewSignature(context actor.Context, msg *messages.Signature) {
	// cs := vth.getConflictSet(string(append(msg.ObjectID, msg.Tip...)))
	// TODO: Save sig, detect done/conflict
}

func (vth *ValidTransactionHandler) getConflictSet(id string) *conflictSet {
	cs, ok := vth.conflictSets[id]
	if !ok {
		cs = newConflictSet(id)
	}
	return cs
}
