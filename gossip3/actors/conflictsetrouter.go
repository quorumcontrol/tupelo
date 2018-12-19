package actors

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/ethereum/go-ethereum/common/hexutil"
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
		context.Forward(csr.getOrCreateCS(context, []byte(msg.ConflictSetID)))
	case *messages.SignatureWrapper:
		context.Forward(csr.getOrCreateCS(context, []byte(msg.ConflictSetID)))
	case *messages.Signature:
		context.Forward(csr.getOrCreateCS(context, []byte(msg.ConflictSetID())))
	case *messages.Store:
		context.Forward(csr.getOrCreateCS(context, msg.Key))
	case *messages.CurrentStateWrapper:
		// TODO: cleanup here
		if parent := context.Parent(); parent != nil {
			context.Forward(context.Parent())
		}
	}
}

func (csr *ConflictSetRouter) getOrCreateCS(context actor.Context, id []byte) *actor.PID {
	idS := hexutil.Encode(id)
	id = []byte(idS)
	cs, ok := csr.conflictSets.Get(id)
	if !ok {
		cs = csr.newConflictSet(context, idS)
		sets, _, _ := csr.conflictSets.Insert(id, cs)
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
