package actors

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/ethereum/go-ethereum/common/hexutil"
	iradix "github.com/hashicorp/go-immutable-radix"
	lru "github.com/hashicorp/golang-lru"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/types"
)

const recentlyDoneConflictCacheSize = 100000

type ConflictSetRouter struct {
	middleware.LogAwareHolder
	recentlyDone *lru.Cache
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
	cache, err := lru.New(recentlyDoneConflictCacheSize)
	if err != nil {
		panic(fmt.Sprintf("error creating LRU cache: %v", err))
	}
	return actor.FromProducer(func() actor.Actor {
		return &ConflictSetRouter{
			conflictSets: iradix.New(),
			cfg:          cfg,
			recentlyDone: cache,
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (csr *ConflictSetRouter) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.TransactionWrapper:
		csr.forwardOrIgnore(context, []byte(msg.ConflictSetID))
	case *messages.SignatureWrapper:
		csr.forwardOrIgnore(context, []byte(msg.ConflictSetID))
	case *messages.Signature:
		csr.forwardOrIgnore(context, []byte(msg.ConflictSetID()))
	case *messages.Store:
		csr.forwardOrIgnore(context, msg.Key)
	case *messages.GetConflictSetView:
		csr.forwardOrIgnore(context, []byte(msg.ConflictSetID))
	case *messages.CurrentStateWrapper:
		if parent := context.Parent(); parent != nil {
			context.Forward(context.Parent())
		}
		if msg.Verified {
			csr.cleanupConflictSet(msg.Key)
		}
	}
}

func (csr *ConflictSetRouter) cleanupConflictSet(id []byte) {
	idS := conflictSetIDToInternalID(id)
	csr.recentlyDone.Add(idS, true)
	sets, csActor, didDelete := csr.conflictSets.Delete([]byte(idS))
	if didDelete {
		csActor.(*actor.PID).Stop()
		csr.conflictSets = sets
	}
}

func (csr *ConflictSetRouter) forwardOrIgnore(context actor.Context, id []byte) {
	cs := csr.getOrCreateCS(context, id)
	if cs != nil {
		context.Forward(cs)
	}
}

// if there is already a conflict set, then forward the message there
// if there isn't then, look at the recently done and if it's done, return nil
// if it's not in either set, then create the actor
func (csr *ConflictSetRouter) getOrCreateCS(context actor.Context, id []byte) *actor.PID {
	idS := conflictSetIDToInternalID(id)
	id = []byte(idS)
	cs, ok := csr.conflictSets.Get(id)
	if !ok {
		_, ok := csr.recentlyDone.Get(idS)
		if ok {
			return nil
		}
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

func conflictSetIDToInternalID(id []byte) string {
	return hexutil.Encode(id)
}
