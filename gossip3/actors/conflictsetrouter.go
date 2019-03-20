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
	"go.uber.org/zap"
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
	return nextHeight(csr.Log, csr.cfg.CurrentStateStore, objectID)
}

func (csr *ConflictSetRouter) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		csr.Log.Debugw("spawning", "self", context.Self().String())
		cfg := csr.cfg
		pool, err := context.SpawnNamed(NewConflictSetWorkerPool(&ConflictSetConfig{
			ConflictSetRouter:  context.Self(),
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
		defer sp.Finish()
		csr.forwardOrIgnore(context, context.Message(), []byte(msg.ConflictSetID))
	case *messages.SignatureWrapper:
		csr.Log.Debugw("forwarding signature wrapper to conflict set", "cs", msg.ConflictSetID)
		csr.forwardOrIgnore(context, context.Message(), []byte(msg.ConflictSetID))
	case *extmsgs.Signature:
		csr.Log.Debugw("forwarding signature to conflict set", "cs", msg.ConflictSetID)
		csr.forwardOrIgnore(context, context.Message(), []byte(msg.ConflictSetID()))
	case *extmsgs.Store:
		csr.Log.Debugw("forwarding store message to conflict set", "key", msg.Key)
		csr.forwardOrIgnore(context, context.Message(), msg.Key)
	case *commitNotification:
		csr.Log.Debugw("received commit notification, computing next transaction height")
		msg.nextHeight = csr.nextHeight(msg.objectID)
		csr.Log.Debugw("forwarding commit notification to conflict set", "store.Key", msg.store.Key,
			"nextHeight", msg.nextHeight)
		csr.forwardOrIgnore(context, context.Message(), msg.store.Key)
	case *messages.CurrentStateWrapper:
		csr.Log.Debugw("received current state wrapper message", "verified", msg.Verified,
			"height", msg.CurrentState.Signature.Height)
		if msg.Verified {
			csr.cleanupConflictSets(msg)
		}
		if parent := context.Parent(); parent != nil {
			csr.Log.Debugw("forwarding current state wrapper message to parent")
			context.Forward(parent)
		}
	case *messages.ActivateSnoozingConflictSets:
		csr.activateSnoozingConflictSets(context, msg.ObjectID)
	case *messages.ValidateTransaction:
		if parent := context.Parent(); parent != nil {
			context.Forward(parent)
		}
	case *messages.SignatureVerification:
		if err := csr.handleSignatureResponse(context, msg); err != nil {
			csr.Log.Errorw("error handling signature response", "err", err)
		}
	case messages.GetNumConflictSets:
		context.Respond(csr.conflictSets.Len())
	}
}

// Clean up conflict sets corresponding to current state and of lower transaction height
func (csr *ConflictSetRouter) cleanupConflictSets(msg *messages.CurrentStateWrapper) {
	csr.Log.Debugw("cleaning up conflict sets", "numSets", csr.conflictSets.Len())
	// For each conflict set corresponding to object ID and up to/including transaction height
	// TODO: Consider if there's a more efficient way of finding stale conflict sets
	objectID := msg.CurrentState.Signature.ObjectID
	for h := uint64(0); h <= msg.CurrentState.Signature.Height; h++ {
		idS := conflictSetIDToInternalID([]byte(extmsgs.ConflictSetID(objectID, h)))
		csr.Log.Debugw("deleting conflict set if it exists", "objectID", objectID, "height", h,
			"id", idS)
		sets, cs, didDelete := csr.conflictSets.Delete([]byte(idS))
		if didDelete {
			csr.recentlyDone.Add(idS, true)
			cs.(*ConflictSet).StopTrace()
			csr.Log.Debugw("deleted conflict set", "id", idS)
			csr.conflictSets = sets
		}
	}
	csr.Log.Debugw("finished cleaning up conflict sets", "numRemaining", csr.conflictSets.Len())
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
	wrapper := ver.Memo.(*messages.CurrentStateWrapper)
	sp := wrapper.NewSpan("handleSignatureResponse")
	csr.Log.Debugw("handleSignatureResponse")
	if ver.Verified {
		sp.SetTag("verified", true)
		wrapper.Verified = true
		csr.forwardOrIgnore(context, wrapper, wrapper.Key)
		sp.Finish()
		return nil
	}
	sp.SetTag("error", true)
	sp.Finish()
	wrapper.StopTrace()
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
	csr.Log.Debugw("determining whether or not to create another conflict set", "numExistingSets",
		csr.conflictSets.Len(), "id", idS)
	cs, ok := csr.conflictSets.Get(id)
	if ok {
		csr.Log.Debugw("got existing conflict set", "cs", idS)
		return cs.(*ConflictSet)
	}

	if _, ok = csr.recentlyDone.Get(idS); ok {
		csr.Log.Debugw("conflict set is already done", "cs", idS)
		return nil
	}

	csr.Log.Debugw("creating conflict set", "cs", idS)
	cs = csr.newConflictSet(idS)
	sets, _, _ := csr.conflictSets.Insert(id, cs)
	csr.conflictSets = sets
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

func nextHeight(log *zap.SugaredLogger, currentStateStore storage.Reader, objectID []byte) uint64 {
	log.Debugw("calculating next height", "objectID", string(objectID))
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
		nh := currState.Signature.Height + 1
		log.Debugw("got object from current state store", "nextHeight", nh)
		return nh
	}

	log.Debugw("couldn't get object from current state store, so returning 0 for next height")
	return 0
}
