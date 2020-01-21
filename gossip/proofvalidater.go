package gossip

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	"github.com/opentracing/opentracing-go"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/messages/v2/build/go/gossip"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/hamtwrapper"
	sigfuncs "github.com/quorumcontrol/tupelo-go-sdk/signatures"
)

// This is used for verifying receiveToken transactions
func verifyRoundConfirmation(rootCtx context.Context, conf *gossip.RoundConfirmation, verKeys []*bls.VerKey) (bool, error) {
	sp, ctx := opentracing.StartSpanFromContext(rootCtx, "verifyRoundConfirmation")
	defer sp.Finish()

	sig := conf.Signature
	msg := conf.RoundCid

	err := sigfuncs.RestoreBLSPublicKey(ctx, sig, verKeys)
	if err != nil {
		sp.SetTag("error", true)
		return false, fmt.Errorf("error restoring public key: %v", err)
	}

	isVerified, err := sigfuncs.Valid(ctx, sig, msg, nil)
	if err != nil {
		sp.SetTag("error", true)
		return false, fmt.Errorf("error verifying round signature: %v", err)
	}
	sp.SetTag("verified", isVerified)
	return isVerified, nil
}

func verifyProof(rootCtx context.Context, msg []byte, proof *gossip.Proof, verKeys []*bls.VerKey) (bool, error) {
	sp, ctx := opentracing.StartSpanFromContext(rootCtx, "validateProof")
	defer sp.Finish()

	validRoundConfirmation, err := verifyRoundConfirmation(ctx, proof.RoundConfirmation, verKeys)
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

	roundCid, err := hamtStore.Put(ctx, proof.Round)
	if err != nil {
		return false, fmt.Errorf("error finding round cid: %v", err)
	}

	roundCidFromConf, err := cid.Cast(proof.RoundConfirmation.RoundCid)
	if err != nil {
		return false, fmt.Errorf("error casting round confirmation roundcid: %v", err)
	}

	if roundCid != roundCidFromConf {
		return false, nil
	}

	checkpointCid, err := hamtStore.Put(ctx, proof.Checkpoint)
	if err != nil {
		return false, fmt.Errorf("error finding checkpoint cid: %v", err)
	}

	checkpointCidFromRound, err := cid.Cast(proof.Round.CheckpointCid)
	if err != nil {
		return false, fmt.Errorf("error casting round checkpoint cid: %v", err)
	}

	if checkpointCid != checkpointCidFromRound {
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

	if abrFound == false {
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

	if tipCid != tipCidFromAbr {
		return false, nil
	}

	objectId := string(proof.ObjectId)

	objectIdFromAbr := string(proof.AddBlockRequest.ObjectId)

	if objectId != objectIdFromAbr {
		return false, nil
	}

	return true, nil
}
