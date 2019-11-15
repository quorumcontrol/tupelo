package actors

import (
	goContext "context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gogo/protobuf/proto"
	datastore "github.com/ipfs/go-datastore"

	"github.com/quorumcontrol/messages/v2/build/go/signatures"
	"github.com/quorumcontrol/tupelo-go-sdk/consensus"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/ethereum/go-ethereum/common/hexutil"
	iradix "github.com/hashicorp/go-immutable-radix"
	lru "github.com/hashicorp/golang-lru"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"go.uber.org/zap"
)

const recentlyDoneConflictCacheSize = 100000

type ConflictSetRouter struct {
	middleware.LogAwareHolder
	recentlyDone    *lru.Cache
	conflictSets    *iradix.Tree
	cfg             *ConflictSetRouterConfig
	pool            *actor.PID
	commitValidator *commitValidator
}

type ConflictSetRouterConfig struct {
	NotaryGroup        *types.NotaryGroup
	Signer             *types.Signer
	SignatureGenerator *actor.PID
	SignatureChecker   *actor.PID
	SignatureSender    *actor.PID
	CurrentStateStore  datastore.Read
	PubSubSystem       remote.PubSub
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

func (csr *ConflictSetRouter) nextHeight(ObjectId []byte) uint64 {
	return nextHeight(csr.Log, csr.cfg.CurrentStateStore, ObjectId)
}

func (csr *ConflictSetRouter) commitTopic() string {
	return csr.cfg.NotaryGroup.Config().CommitTopic
}

// this function is its own actor
func (csr *ConflictSetRouter) commitopicHandler(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		commitTopic := csr.commitTopic()
		cfg := csr.cfg

		topicValidator := newCommitValidator(cfg.NotaryGroup, cfg.SignatureChecker)
		err := cfg.PubSubSystem.RegisterTopicValidator(commitTopic, topicValidator.validate, pubsub.WithValidatorTimeout(1500*time.Millisecond))
		if err != nil {
			panic(fmt.Sprintf("error registering topic validator: %v", err))
		}
		csr.commitValidator = topicValidator

		_, err = context.SpawnNamed(cfg.PubSubSystem.NewSubscriberProps(commitTopic), "commit-subscriber")
		if err != nil {
			panic(fmt.Sprintf("error spawning commit receiver: %v", err))
		}
	case *actor.Stopping:
		csr.cfg.PubSubSystem.UnregisterTopicValidator(csr.commitTopic())
	case *signatures.TreeState:
		wrapper := &messages.CurrentStateWrapper{
			CurrentState: msg,
			Verified:     true, // because it came in through a validated pubsub channel
			Metadata:     messages.MetadataMap{"seen": time.Now()},
			NextHeight:   csr.nextHeight(msg.ObjectId),
		}

		context.Send(context.Parent(), wrapper)
	}
}

func (csr *ConflictSetRouter) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		csr.Log.Debugw("spawning", "self", context.Self().String())
		cfg := csr.cfg

		// handle the incoming confirmed TreeStates differently
		context.Spawn(actor.PropsFromFunc(csr.commitopicHandler))

		pool, err := context.SpawnNamed(NewConflictSetWorkerPool(&ConflictSetConfig{
			ConflictSetRouter:  context.Self(),
			NotaryGroup:        cfg.NotaryGroup,
			Signer:             cfg.Signer,
			SignatureGenerator: cfg.SignatureGenerator,
			SignatureSender:    cfg.SignatureSender,
			CommitValidator:    csr.commitValidator,
		}), "csrPool")
		if err != nil {
			panic(fmt.Errorf("error spawning csrPool: %v", err))
		}
		csr.pool = pool

	case *messages.TransactionWrapper:
		csr.Log.Debugw("received TransactionWrapper message")
		sp := msg.NewSpan("conflictset-router")
		defer sp.Finish()
		csr.forwardOrIgnore(context, msg, msg.Transaction.ObjectId, msg.Transaction.Height)
	case *messages.SignatureWrapper:
		csr.Log.Debugw("forwarding signature wrapper to conflict set", "cs", msg.ConflictSetID)
		csr.forwardOrIgnore(context, msg, msg.State.ObjectId, msg.State.Height)
	case *signatures.TreeState:
		csr.Log.Debugw("forwarding TreeState to conflict set", "cs", consensus.ConflictSetID(msg.ObjectId, msg.Height))
		csr.forwardOrIgnore(context, msg, msg.ObjectId, msg.Height)
	case *messages.CurrentStateWrapper:
		csr.handleCurrentStateWrapper(context, msg)
	case *messages.ImportCurrentState:
		currStateWrapper := &messages.CurrentStateWrapper{
			Internal:     false,
			Verified:     csr.commitValidator.validate(goContext.Background(), "", msg.CurrentState),
			CurrentState: msg.CurrentState,
			Metadata:     messages.MetadataMap{"seen": time.Now()},
		}
		if currStateWrapper.Verified {
			csr.handleCurrentStateWrapper(context, currStateWrapper)
		}
	case *messages.ActivateSnoozingConflictSets:
		csr.Log.Debugw("csr received activate snoozed")
		csr.activateSnoozingConflictSets(context, msg.ObjectId)
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

func (csr *ConflictSetRouter) handleCurrentStateWrapper(context actor.Context, msg *messages.CurrentStateWrapper) {
	csr.Log.Debugw("received current state wrapper message", "verified", msg.Verified,
		"height", msg.CurrentState.Height, "internal", msg.Internal)

	if msg.Internal {
		if err := csr.cfg.PubSubSystem.Broadcast(csr.commitTopic(), msg.CurrentState); err != nil {
			csr.Log.Errorw("error publishing", "err", err)
		}
	}

	csr.cleanupConflictSets(msg)
	if parent := context.Parent(); parent != nil {
		csr.Log.Debugw("forwarding current state wrapper message to parent")
		context.Send(parent, msg)
	}
}

// Clean up conflict sets corresponding to current state and of lower transaction height
func (csr *ConflictSetRouter) cleanupConflictSets(msg *messages.CurrentStateWrapper) {
	numConflictSets := csr.conflictSets.Len()
	if numConflictSets == 0 {
		csr.Log.Debugw("no conflict sets to clean up")
		return
	}

	curHeight := msg.CurrentState.Height
	objectID := msg.CurrentState.ObjectId
	csr.Log.Debugw("cleaning up conflict sets", "numSets", numConflictSets, "ObjectId",
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
		csr.Log.Debugw("deleting conflict set", "ObjectId", string(objectID), "height", height)
		idS := conflictSetIDToInternalID([]byte(consensus.ConflictSetID(objectID, uint64(height))))
		csr.recentlyDone.Add(idS, true)
		cs.StopTrace()
		txn.Delete(k)
		return false
	})
	csr.conflictSets = txn.Commit()

	csr.Log.Debugw("finished cleaning up conflict sets", "numRemaining", csr.conflictSets.Len())
}

func (csr *ConflictSetRouter) forwardOrIgnore(context actor.Context, msg interface{},
	objectId []byte, height uint64) {
	cs := csr.getOrCreateCS(objectId, height)
	if cs == nil {
		csr.Log.Debugw("ignoring message")
		return
	}

	csr.Log.Debugw("sending conflict set worker request to pool")
	context.Send(csr.pool, &csWorkerRequest{
		msg: msg,
		cs:  cs,
	})
}

func (csr *ConflictSetRouter) handleSignatureResponse(context actor.Context, ver *messages.SignatureVerification) error {
	wrapper := ver.Memo.(*messages.CurrentStateWrapper)
	sp := wrapper.NewSpan("handleSignatureResponse")
	csr.Log.Debugw("handleSignatureResponse")
	if ver.Verified {
		sp.SetTag("verified", true)
		wrapper.Verified = true
		csr.forwardOrIgnore(context, wrapper, wrapper.CurrentState.ObjectId,
			wrapper.CurrentState.Height)
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
func (csr *ConflictSetRouter) getOrCreateCS(objectId []byte, height uint64) *ConflictSet {
	idS := conflictSetIDToInternalID([]byte(consensus.ConflictSetID(objectId, height)))
	csr.Log.Debugw("determining whether or not to create another conflict set", "numExistingSets",
		csr.conflictSets.Len(), "objectId", objectId, "height", height)
	nodeID := []byte(fmt.Sprintf("%s/%d", objectId, height))
	csI, ok := csr.conflictSets.Get(nodeID)
	if ok {
		csr.Log.Debugw("got existing conflict set", "objectId", objectId, "height", height)
		return csI.(*ConflictSet)
	}

	if _, ok = csr.recentlyDone.Get(idS); ok {
		csr.Log.Debugw("conflict set is already done", "objectId", objectId, "height", height)
		return nil
	}

	csr.Log.Debugw("creating conflict set", "objectId", objectId, "height", height)
	cfg := csr.cfg
	cs := newConflictSet(idS)
	sp := cs.StartTrace("conflictset")
	sp.SetTag("csid", idS)
	sp.SetTag("objectId", objectId)
	sp.SetTag("height", height)
	sp.SetTag("signer", cfg.Signer.ID)
	sets, _, _ := csr.conflictSets.Insert(nodeID, cs)
	csr.conflictSets = sets
	return cs
}

func (csr *ConflictSetRouter) activateSnoozingConflictSets(context actor.Context, ObjectId []byte) {
	nh := csr.nextHeight(ObjectId)
	nodeID := fmt.Sprintf("%s/%d", ObjectId, nh)
	cs, ok := csr.conflictSets.Get([]byte(nodeID))
	if !ok {
		csr.Log.Debugw("no conflict sets to desnooze", "ObjectId", ObjectId, "height", nh)
		return
	}

	csr.Log.Debugw("activating snoozed", "cs", nodeID)
	context.Send(csr.pool, &csWorkerRequest{
		cs:  cs.(*ConflictSet),
		msg: context.Message(),
	})
}

func conflictSetIDToInternalID(id []byte) string {
	return hexutil.Encode(id)
}

func nextHeight(log *zap.SugaredLogger, currentStateStore datastore.Read, objectId []byte) uint64 {
	log.Debugw("calculating next height", "ObjectId", string(objectId))
	currStateBits, err := currentStateStore.Get(datastore.NewKey(string(objectId)))
	if err != nil && err != datastore.ErrNotFound {
		panic(fmt.Errorf("error getting current state: %v", err))
	}
	if len(currStateBits) > 0 {
		currState := &signatures.TreeState{}
		err = proto.Unmarshal(currStateBits, currState)
		if err != nil {
			panic(fmt.Errorf("error unmarshaling: %v", err))
		}
		nh := currState.Height + 1
		log.Debugw("got object from current state store", "nextHeight", nh)
		return nh
	}

	log.Debugw("couldn't get object from current state store, so returning 0 for next height")
	return 0
}
