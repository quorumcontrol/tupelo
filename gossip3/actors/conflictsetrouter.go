package actors

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/ethereum/go-ethereum/common/hexutil"
	iradix "github.com/hashicorp/go-immutable-radix"
	lru "github.com/hashicorp/golang-lru"
	"github.com/quorumcontrol/storage"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/types"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
)

const recentlyDoneConflictCacheSize = 100000

type ConflictSetRouter struct {
	middleware.LogAwareHolder
	recentlyDone *lru.Cache
	conflictSets *iradix.Tree
	cfg          *ConflictSetRouterConfig
	pool         *actor.PID
}

type ConflictSetRouterConfig struct {
	NotaryGroup        *types.NotaryGroup
	Signer             *types.Signer
	SignatureGenerator *actor.PID
	SignatureChecker   *actor.PID
	SignatureSender    *actor.PID
	CurrentStateStore  storage.Reader
}

type commitNotification struct {
	store      *extmsgs.Store
	objectID   []byte
	height     uint64
	nextHeight uint64
}

func NewConflictSetRouterProps(cfg *ConflictSetRouterConfig) *actor.Props {
	cache, err := lru.New(recentlyDoneConflictCacheSize)
	if err != nil {
		panic(fmt.Sprintf("error creating LRU cache: %v", err))
	}
	return actor.PropsFromProducer(func() actor.Actor {
		return &ConflictSetRouter{
			conflictSets: iradix.New(),
			cfg:          cfg,
			recentlyDone: cache,
		}
	}).WithReceiverMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (csr *ConflictSetRouter) nextHeight(objectID []byte) uint64 {
	return nextHeight(csr.cfg.CurrentStateStore, objectID)
}

func (csr *ConflictSetRouter) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		csr.Log.Debugw("spawning", "self", context.Self().String())
		cfg := csr.cfg
		pool, err := context.SpawnNamed(NewConflictSetWorkerPool(&ConflictSetConfig{
			ConflictSetRouter:  context.Self(),
			CurrentStateStore:  cfg.CurrentStateStore,
			NotaryGroup:        cfg.NotaryGroup,
			Signer:             cfg.Signer,
			SignatureChecker:   cfg.SignatureChecker,
			SignatureGenerator: cfg.SignatureGenerator,
			SignatureSender:    cfg.SignatureSender,
		}), "csrPool")
		if err != nil {
			panic(fmt.Errorf("error spawning csrPool: %v", err))
		}
		csr.pool = pool
	case *messages.TransactionWrapper:
		sp := msg.NewSpan("conflictset-router")
		csr.forwardOrIgnore(context, context.Message(), []byte(msg.ConflictSetID))
		sp.Finish()
	case *messages.SignatureWrapper:
		csr.forwardOrIgnore(context, context.Message(), []byte(msg.ConflictSetID))
	case *extmsgs.Signature:
		csr.forwardOrIgnore(context, context.Message(), []byte(msg.ConflictSetID()))
	case *extmsgs.Store:
		csr.forwardOrIgnore(context, context.Message(), msg.Key)
	case *commitNotification:
		msg.nextHeight = csr.nextHeight(msg.objectID)
		csr.forwardOrIgnore(context, context.Message(), msg.store.Key)
	case *messages.CurrentStateWrapper:
		if parent := context.Parent(); parent != nil {
			context.Forward(parent)
		}
		if msg.Verified {
			csr.cleanupConflictSet(msg.Key)
		}
	case *messages.ActivateSnoozingConflictSets:
		csr.activateSnoozingConflictSets(context, msg.ObjectID)
	case *messages.ValidateTransaction:
		if parent := context.Parent(); parent != nil {
			context.Forward(parent)
		}
	case *messages.SignatureVerification:
		csr.handleSignatureResponse(context, msg)
	}
}

func (csr *ConflictSetRouter) cleanupConflictSet(id []byte) {
	csr.Log.Debugw("cleaning up conflict set", "cs", id)
	idS := conflictSetIDToInternalID(id)
	csr.recentlyDone.Add(idS, true)
	sets, cs, didDelete := csr.conflictSets.Delete([]byte(idS))
	if didDelete {
		cs.(*ConflictSet).StopTrace()
		csr.conflictSets = sets
	}
}

func (csr *ConflictSetRouter) forwardOrIgnore(context actor.Context, msg interface{}, id []byte) {
	cs := csr.getOrCreateCS(id)
	if cs != nil {
		context.Send(csr.pool, &csWorkerRequest{
			msg: msg,
			cs:  cs,
		})
	}
}

func (csr *ConflictSetRouter) handleSignatureResponse(context actor.Context, ver *messages.SignatureVerification) error {
	csr.Log.Debugw("handleSignatureResponse")
	if ver.Verified {
		wrapper := ver.Memo.(*messages.CurrentStateWrapper)
		wrapper.Verified = true
		csr.forwardOrIgnore(context, wrapper, wrapper.Key)
		return nil
	}
	csr.Log.Errorw("invalid signature")
	// for now we can just ignore
	return nil
}

// if there is already a conflict set, then forward the message there
// if there isn't then, look at the recently done and if it's done, return nil
// if it's not in either set, then create the actor
func (csr *ConflictSetRouter) getOrCreateCS(id []byte) *ConflictSet {
	idS := conflictSetIDToInternalID(id)
	id = []byte(idS)
	cs, ok := csr.conflictSets.Get(id)
	if !ok {
		_, ok := csr.recentlyDone.Get(idS)
		if ok {
			return nil
		}
		csr.Log.Debugw("creating conflict set", "cs", idS)
		cs = csr.newConflictSet(idS)
		sets, _, _ := csr.conflictSets.Insert(id, cs)
		csr.conflictSets = sets
	}
	return cs.(*ConflictSet)
}

func (csr *ConflictSetRouter) newConflictSet(id string) *ConflictSet {
	csr.Log.Debugw("new conflict set", "id", id)
	cfg := csr.cfg
	cs := NewConflictSet(id)
	sp := cs.StartTrace("conflictset")
	sp.SetTag("csid", id)
	sp.SetTag("signer", cfg.Signer.ID)
	return cs
}

func (csr *ConflictSetRouter) activateSnoozingConflictSets(context actor.Context, objectID []byte) {
	conflictSetID := extmsgs.ConflictSetID(objectID, csr.nextHeight(objectID))

	cs, ok := csr.conflictSets.Get([]byte(conflictSetIDToInternalID([]byte(conflictSetID))))
	if ok {
		csr.Log.Debugw("activating snoozed", "cs", conflictSetIDToInternalID([]byte(conflictSetID)))
		context.Send(csr.pool, &csWorkerRequest{
			cs:  cs.(*ConflictSet),
			msg: context.Message(),
		})
	}
}

func conflictSetIDToInternalID(id []byte) string {
	return hexutil.Encode(id)
}

func nextHeight(currentStateStore storage.Reader, objectID []byte) uint64 {
	currStateBits, err := currentStateStore.Get(objectID)
	if err != nil {
		panic(fmt.Errorf("error getting current state: %v", err))
	}
	if len(currStateBits) > 0 {
		var currState extmsgs.CurrentState
		_, err = currState.UnmarshalMsg(currStateBits)
		if err != nil {
			panic(fmt.Errorf("error unmarshaling: %v", err))
		}
		return currState.Signature.Height + 1
	} else {
		return 0
	}
}
