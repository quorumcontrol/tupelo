package proof

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	"github.com/opentracing/opentracing-go"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/gossip"
	"github.com/quorumcontrol/tupelo/sdk/bls"
	"github.com/quorumcontrol/tupelo/sdk/gossip/hamtwrapper"
	sigfuncs "github.com/quorumcontrol/tupelo/sdk/signatures"
)

func verifyRoundConfirmation(rootCtx context.Context, conf *gossip.RoundConfirmation, quorumCount uint64, verKeys []*bls.VerKey) (bool, error) {
	sp, ctx := opentracing.StartSpanFromContext(rootCtx, "verifyRoundConfirmation")
	defer sp.Finish()

	sig := conf.Signature
	if uint64(sigfuncs.SignerCount(sig)) < quorumCount {
		return false, nil
	}

	err := sigfuncs.RestoreBLSPublicKey(ctx, sig, verKeys)
	if err != nil {
		sp.SetTag("error", true)
		return false, fmt.Errorf("error restoring public key: %v", err)
	}

	msg := conf.RoundCid
	isVerified, err := sigfuncs.Valid(ctx, sig, msg, nil)
	if err != nil {
		sp.SetTag("error", true)
		return false, fmt.Errorf("error verifying round signature: %v", err)
	}
	sp.SetTag("verified", isVerified)
	return isVerified, nil
}

func Verify(rootCtx context.Context, proof *gossip.Proof, quorumCount uint64, verKeys []*bls.VerKey) (bool, error) {
	sp, ctx := opentracing.StartSpanFromContext(rootCtx, "validateProof")
	defer sp.Finish()

	sw := safewrap.SafeWrap{}

	validRoundConfirmation, err := verifyRoundConfirmation(ctx, proof.RoundConfirmation, quorumCount, verKeys)
	if err != nil {
		return false, fmt.Errorf("error verifying round confirmation: %v", err)
	}

	if !validRoundConfirmation {
		return false, nil
	}

	underlyingStore := nodestore.MustMemoryStore(ctx)
	hamtStore := hamt.CborIpldStore{
		Blocks: hamtwrapper.NewStore(underlyingStore),
	}

	roundCid := sw.WrapObject(proof.Round).Cid()

	roundCidFromConf, err := cid.Cast(proof.RoundConfirmation.RoundCid)
	if err != nil {
		return false, fmt.Errorf("error casting round confirmation roundcid: %v", err)
	}

	if !(roundCid.Equals(roundCidFromConf)) {
		return false, nil
	}

	checkpointCid := sw.WrapObject(proof.Checkpoint).Cid()

	checkpointCidFromRound, err := cid.Cast(proof.Round.CheckpointCid)
	if err != nil {
		return false, fmt.Errorf("error casting round checkpoint cid: %v", err)
	}

	if !(checkpointCid.Equals(checkpointCidFromRound)) {
		return false, nil
	}

	abrCid, err := hamtStore.Put(ctx, proof.AddBlockRequest)
	if err != nil {
		return false, fmt.Errorf("error finding add block request cid: %v", err)
	}

	abrFound := false
	for _, cpAbrCidBytes := range proof.Checkpoint.AddBlockRequests {
		cpAbrCid, err := cid.Cast(cpAbrCidBytes)
		if err != nil {
			return false, fmt.Errorf("error casting checkpoint add block request cid: %v", err)
		}

		if cpAbrCid == abrCid {
			abrFound = true
			break
		}
	}

	if !abrFound {
		return false, nil
	}

	tipCid, err := cid.Cast(proof.Tip)
	if err != nil {
		return false, fmt.Errorf("error casting tip cid: %v", err)
	}

	tipCidFromAbr, err := cid.Cast(proof.AddBlockRequest.NewTip)
	if err != nil {
		return false, fmt.Errorf("error casting add block request new tip cid: %v", err)
	}

	if !(tipCid.Equals(tipCidFromAbr)) {
		return false, nil
	}

	objectId := string(proof.ObjectId)

	objectIdFromAbr := string(proof.AddBlockRequest.ObjectId)

	if objectId != objectIdFromAbr {
		return false, nil
	}

	if sw.Err != nil {
		return false, fmt.Errorf("error decoding object: %v", sw.Err)
	}

	return true, nil
}
