package actors

import (
	"context"
	"fmt"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/Workiva/go-datastructures/bitarray"
	lru "github.com/hashicorp/golang-lru"
	peer "github.com/libp2p/go-libp2p-peer"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/types"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"go.uber.org/zap"
)

// I think if we had a validator that a) made sure the only publisher was the notary group, b) the commit is valid (sigs and stuff) and c) wasn't a repeat commit that had already been seen that we could move the incoming commit validation work to the gossipsub
// and then in the app layer we could just treat commit messages as "good"

// the commitValidator is a pubsub validator that makes sure the signatures are correct
type commitValidator struct {
	notaryGroup      *types.NotaryGroup
	signatureChecker *actor.PID
	log              *zap.SugaredLogger
	seen             *lru.Cache
}

func newCommitValidator(group *types.NotaryGroup, sigChecker *actor.PID) *commitValidator {
	cache, err := lru.New(10000)
	if err != nil {
		panic(fmt.Errorf("error creating commit cache for validator: %v", err))
	}
	return &commitValidator{
		notaryGroup:      group,
		signatureChecker: sigChecker,
		log:              middleware.Log.Named("commitValidator"),
		seen:             cache,
	}
}

func (cv *commitValidator) validate(ctx context.Context, p peer.ID, msg extmsgs.WireMessage) bool {
	currState, ok := msg.(*extmsgs.CurrentState)
	if !ok {
		cv.log.Errorw("received non-currentstate message")
		return false
	}

	if cv.seen.Contains(cacheKey(currState)) {
		cv.log.Infow("stopping propogation of already-seen message")
		return false
	}

	sig := currState.Signature
	signerArray, err := bitarray.Unmarshal(sig.Signers)
	if err != nil {
		cv.log.Errorw("error unmarshaling signer bit array")
		return false
	}
	var verKeys [][]byte

	signers := cv.notaryGroup.AllSigners()
	for i, signer := range signers {
		isSet, err := signerArray.GetBit(uint64(i))
		if err != nil {
			// return fmt.Errorf("error getting bit: %v", err)
			return false
		}
		if isSet {
			verKeys = append(verKeys, signer.VerKey.Bytes())
		}
	}

	if uint64(len(verKeys)) < cv.notaryGroup.QuorumCount() {
		cv.log.Infow("too few signatures on commit message")
		return false
	}

	cv.log.Debugw("checking signature", "numVerKeys", len(verKeys))
	actorContext := actor.EmptyRootContext

	fut := actorContext.RequestFuture(cv.signatureChecker, &messages.SignatureVerification{
		Message:   sig.GetSignable(),
		Signature: sig.Signature,
		VerKeys:   verKeys,
	}, 1*time.Second)

	res, err := fut.Result()
	if err != nil {
		cv.log.Errorw("error getting signature verification")
		return false
	}

	if res.(*messages.SignatureVerification).Verified {
		cv.seen.Add(cacheKey(currState), struct{}{})
		return true
	}

	cv.log.Infow("unknown failure fallthrough")

	return false
}

func cacheKey(currState *extmsgs.CurrentState) string {
	return string(append(currState.Signature.ObjectID, currState.Signature.NewTip...))
}
