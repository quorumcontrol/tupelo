package actors

import (
	"fmt"
	"strconv"
	"strings"
	"time"

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
		csr.forwardOrIgnore(context, msg, msg.Transaction.ObjectID,
			msg.Transaction.Height)
	case *messages.SignatureWrapper:
		csr.Log.Debugw("forwarding signature wrapper to conflict set", "cs", msg.ConflictSetID)
		csr.forwardOrIgnore(context, msg, msg.Signature.ObjectID, msg.Signature.Height)
	case *extmsgs.Signature:
		csr.Log.Debugw("forwarding signature to conflict set", "cs", msg.ConflictSetID())
		csr.forwardOrIgnore(context, msg, msg.ObjectID, msg.Height)
	case *extmsgs.CurrentState:
		wrapper := &messages.CurrentStateWrapper{
			CurrentState: msg,
			Internal:     false,
			Verified:     true,
			Metadata:     messages.MetadataMap{"seen": time.Now()},
			NextHeight:   csr.nextHeight(msg.Signature.ObjectID),
		}

		csr.forwardOrIgnore(context, wrapper, msg.Signature.ObjectID, msg.Signature.Height)
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
		csr.Log.Debugw("csr received activate snoozed")
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
	numConflictSets := csr.conflictSets.Len()
	if numConflictSets == 0 {
		csr.Log.Debugw("no conflict sets to clean up")
		return
	}

	curHeight := msg.CurrentState.Signature.Height
	objectID := msg.CurrentState.Signature.ObjectID
	csr.Log.Debugw("cleaning up conflict sets", "numSets", numConflictSets, "objectID",
		string(objectID), "currentHeight", curHeight)

	// Delete conflict sets for object ID and with height <= current one
	txn := csr.conflictSets.Txn()
	root := csr.conflictSets.Root()
	root.WalkPrefix([]byte(fmt.Sprintf("%s/", objectID)), func(k []byte, v interface{}) bool {
		kS := string(k)
		splitKey := strings.SplitN(kS, "/", 2)
		heightS := splitKey[len(splitKey)-1]
		height, err := strconv.ParseInt(heightS, 10, 64)
		if err != nil {
			csr.Log.Errorw("invalid key in csr.conflictSets", "key", string(k), "split", splitKey,
				"height", heightS, "err", err)
			return false
		}

		if uint64(height) > curHeight {
			// Higher than the current height, no need to prune
			return false
		}

		// Prune as of height less than or equal to current height
		cs := v.(*ConflictSet)
		csr.Log.Debugw("deleting conflict set", "objectID", string(objectID), "height", height)
		idS := conflictSetIDToInternalID([]byte(extmsgs.ConflictSetID(objectID, uint64(height))))
		csr.recentlyDone.Add(idS, true)
		cs.StopTrace()
		txn.Delete(k)
		return false
	})
	csr.conflictSets = txn.Commit()

	csr.Log.Debugw("finished cleaning up conflict sets", "numRemaining", csr.conflictSets.Len())
}

func (csr *ConflictSetRouter) forwardOrIgnore(context actor.Context, msg interface{},
	objectID []byte, height uint64) {
	cs := csr.getOrCreateCS(objectID, height)
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
		csr.forwardOrIgnore(context, wrapper, wrapper.CurrentState.Signature.ObjectID,
			wrapper.CurrentState.Signature.Height)
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
func (csr *ConflictSetRouter) getOrCreateCS(objectID []byte, height uint64) *ConflictSet {
	idS := conflictSetIDToInternalID([]byte(extmsgs.ConflictSetID(objectID, height)))
	csr.Log.Debugw("determining whether or not to create another conflict set", "numExistingSets",
		csr.conflictSets.Len(), "objectID", objectID, "height", height)
	nodeID := []byte(fmt.Sprintf("%s/%d", objectID, height))
	csI, ok := csr.conflictSets.Get(nodeID)
	if ok {
		csr.Log.Debugw("got existing conflict set", "objectID", objectID, "height", height)
		return csI.(*ConflictSet)
	}

	if _, ok = csr.recentlyDone.Get(idS); ok {
		csr.Log.Debugw("conflict set is already done", "objectID", objectID, "height", height)
		return nil
	}

	csr.Log.Debugw("creating conflict set", "objectID", objectID, "height", height)
	cfg := csr.cfg
	cs := NewConflictSet(idS)
	sp := cs.StartTrace("conflictset")
	sp.SetTag("csid", idS)
	sp.SetTag("objectID", objectID)
	sp.SetTag("height", height)
	sp.SetTag("signer", cfg.Signer.ID)
	sets, _, _ := csr.conflictSets.Insert(nodeID, cs)
	csr.conflictSets = sets
	return cs
}

func (csr *ConflictSetRouter) activateSnoozingConflictSets(context actor.Context, objectID []byte) {
	nodeID := []byte(fmt.Sprintf("%s/%d", objectID, csr.nextHeight(objectID)))
	cs, ok := csr.conflictSets.Get(nodeID)
	if ok {
		csr.Log.Debugw("activating snoozed", "cs", nodeID)
		context.Send(csr.pool, &csWorkerRequest{
			cs:  cs.(*ConflictSet),
			msg: context.Message(),
		})
		return
	}
	csr.Log.Debugw("no conflict sets to desnooze", "objectID", objectID, "height", nextHeight)
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
