package actors

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/hashicorp/go-immutable-radix"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
)

type ConflictSetRouter struct {
	conflictSets *iradix.Tree
}

func NewConflictSetRouterProps() *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &ConflictSetRouter{
			conflictSets: iradix.New(),
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (csr *ConflictSetRouter) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.TransactionWrapper:
		csr.handleNewTransaction(context, msg)
	}
}

func (csr *ConflictSetRouter) handleNewTransaction(context actor.Context, msg *messages.TransactionWrapper) {
	cs, ok := csr.conflictSets.Get([]byte(msg.ConflictSetID))
	if !ok {
		cs = csr.newConflictSet(context, msg.ConflictSetID)
	}
	context.Forward(cs.(*actor.PID))
}

func (csr *ConflictSetRouter) newConflictSet(context actor.Context, id string) *actor.PID {
	cs, err := context.SpawnNamed(NewConflictSetProps(id), id)
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}
	return cs
}
