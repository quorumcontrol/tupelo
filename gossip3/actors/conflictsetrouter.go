package actors

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/hashicorp/go-immutable-radix"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/types"
)

type ConflictSetRouter struct {
	middleware.LogAwareHolder

	conflictSets *iradix.Tree
	cfg          *ConflictSetRouterConfig
}

type ConflictSetRouterConfig struct {
	NotaryGroup        *types.NotaryGroup
	Signer             *types.Signer
	SignatureGenerator *actor.PID
	SignatureChecker   *actor.PID
	SignatureSender    *actor.PID
}

func NewConflictSetRouterProps(cfg *ConflictSetRouterConfig) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &ConflictSetRouter{
			conflictSets: iradix.New(),
			cfg:          cfg,
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
	case *messages.SignatureWrapper:
		csr.handleNewSignature(context, msg)
	case *messages.CurrentStateWrapper:
		if parent := context.Parent(); parent != nil {
			context.Forward(context.Parent())
		}
	}
}

func (csr *ConflictSetRouter) handleNewTransaction(context actor.Context, msg *messages.TransactionWrapper) {
	csr.Log.Debugw("new transaction")
	context.Forward(csr.getOrCreateCS(context, msg.ConflictSetID))
}

func (csr *ConflictSetRouter) handleNewSignature(context actor.Context, msg *messages.SignatureWrapper) {
	csr.Log.Debugw("new signature")
	context.Forward(csr.getOrCreateCS(context, msg.ConflictSetID))
}

func (csr *ConflictSetRouter) getOrCreateCS(context actor.Context, id string) *actor.PID {
	cs, ok := csr.conflictSets.Get([]byte(id))
	if !ok {
		cs = csr.newConflictSet(context, id)
		sets, _, _ := csr.conflictSets.Insert([]byte(id), cs)
		csr.conflictSets = sets
	}
	return cs.(*actor.PID)
}

func (csr *ConflictSetRouter) newConflictSet(context actor.Context, id string) *actor.PID {
	csr.Log.Debugw("new conflict set", "id", id)
	cfg := csr.cfg
	cs, err := context.SpawnNamed(NewConflictSetProps(&ConflictSetConfig{
		ID:                 id,
		NotaryGroup:        cfg.NotaryGroup,
		Signer:             cfg.Signer,
		SignatureChecker:   cfg.SignatureChecker,
		SignatureGenerator: cfg.SignatureGenerator,
		SignatureSender:    cfg.SignatureSender,
	}), id)
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}
	return cs
}
