package gossip

import (
	"context"
	"fmt"

	"github.com/opentracing/opentracing-go"
	"github.com/quorumcontrol/messages/build/go/gossip"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
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

	return false, nil
}
