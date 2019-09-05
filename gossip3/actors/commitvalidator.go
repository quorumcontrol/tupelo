package actors

import (
	"context"
	"fmt"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/quorumcontrol/messages/build/go/signatures"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/tupelo-go-sdk/consensus"
	sigfuncs "github.com/quorumcontrol/tupelo-go-sdk/signatures"

	"github.com/AsynkronIT/protoactor-go/actor"
	lru "github.com/hashicorp/golang-lru"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"go.uber.org/zap"
)

// the commitValidator is a pubsub validator that makes sure that
// a currentState message on this topic is valid
type commitValidator struct {
	notaryGroup      *types.NotaryGroup
	verKeys          []*bls.VerKey
	quorumCount      uint64
	signatureChecker *actor.PID
	log              *zap.SugaredLogger
	seen             *lru.Cache
}

func newCommitValidator(group *types.NotaryGroup, sigChecker *actor.PID) *commitValidator {
	cache, err := lru.New(10000)
	if err != nil {
		panic(fmt.Errorf("error creating commit cache for validator: %v", err))
	}
	signers := group.AllSigners()
	verKeys := make([]*bls.VerKey, len(signers))
	for i, signer := range signers {
		verKeys[i] = signer.VerKey
	}
	return &commitValidator{
		notaryGroup:      group,
		verKeys:          verKeys,
		quorumCount:      group.QuorumCount(),
		signatureChecker: sigChecker,
		log:              middleware.Log.Named("commitValidator"),
		seen:             cache,
	}
}

func (cv *commitValidator) validate(ctx context.Context, p peer.ID, msg proto.Message) bool {
	currState, ok := msg.(*signatures.TreeState)
	if !ok {
		cv.log.Errorw("received non-currentstate message")
		return false
	}

	// if we've seen this currentState before, we can just
	// return what we've already seen.
	val, ok := cv.seen.Get(cacheKey(currState))
	if ok {
		return val.(bool)
	}

	sig := currState.Signature

	if uint64(sigfuncs.SignerCount(sig)) < cv.quorumCount {
		cv.log.Infow("too few signatures on commit message", "lenVerKeys", sigfuncs.SignerCount(sig), "quorumAt", cv.quorumCount)
		cv.seen.Add(cacheKey(currState), false)
		return false
	}

	cv.log.Debugw("checking signature")
	actorContext := actor.EmptyRootContext

	fut := actorContext.RequestFuture(cv.signatureChecker, &messages.SignatureVerification{
		Message:   consensus.GetSignable(currState),
		Signature: sig,
		VerKeys:   cv.verKeys,
	}, 1*time.Second)

	res, err := fut.Result()
	if err != nil {
		cv.log.Errorw("error getting signature verification", "err", err)
		// specifically do not cache this particular false because it could
		// be a timeout.
		return false
	}

	if res.(*messages.SignatureVerification).Verified {
		cv.seen.Add(cacheKey(currState), true)
		return true
	}

	cv.log.Infow("unknown failure fallthrough")

	cv.seen.Add(cacheKey(currState), false)
	return false
}

func cacheKey(currState *signatures.TreeState) string {
	return string(currState.Signature.Signature) + string(consensus.GetSignable(currState))
}
