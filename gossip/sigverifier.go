package gossip

import (
	"context"
	"fmt"

	"github.com/quorumcontrol/tupelo-go-sdk/bls"

	"github.com/quorumcontrol/messages/v2/build/go/signatures"
	sigfuncs "github.com/quorumcontrol/tupelo-go-sdk/signatures"

	"github.com/opentracing/opentracing-go"
)

// This is used for verifying receiveToken transactions
// TODO: we want receive tokens to use round paths through hamts intead of signatures
func verifySignature(rootCtx context.Context, msg []byte, signature *signatures.Signature, verKeys []*bls.VerKey) (bool, error) {
	sp, ctx := opentracing.StartSpanFromContext(rootCtx, "verifySignature")
	defer sp.Finish()

	err := sigfuncs.RestoreBLSPublicKey(ctx, signature, verKeys)
	if err != nil {
		sp.SetTag("error", true)
		// logger.Errorf("error restoring public key %v", err)
		return false, fmt.Errorf("error verifying: %v", err)
	}

	isVerified, err := sigfuncs.Valid(ctx, signature, msg, nil)
	if err != nil {
		sp.SetTag("error", true)
		// logger.Errorf("error verifying %v", err)
		return false, fmt.Errorf("error verifying: %v", err)
	}
	sp.SetTag("verified", isVerified)
	return true, nil
}
