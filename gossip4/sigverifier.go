package gossip4

import (
	"context"
	"fmt"

	"github.com/quorumcontrol/tupelo-go-sdk/bls"

	"github.com/quorumcontrol/messages/build/go/signatures"
	sigfuncs "github.com/quorumcontrol/tupelo-go-sdk/signatures"

	"github.com/opentracing/opentracing-go"
)

func verifySignature(ctx context.Context, msg []byte, signature *signatures.Signature, verKeys []*bls.VerKey) (bool, error) {
	sp := opentracing.SpanFromContext(ctx)
	defer sp.Finish()

	logger.Debugf("handle signature verification")

	err := sigfuncs.RestoreBLSPublicKey(signature, verKeys)
	if err != nil {
		sp.SetTag("error", true)
		logger.Errorf("error restoring public key", "err", err)
		return false, fmt.Errorf("error verifying: %v", err)
	}

	isVerified, err := sigfuncs.Valid(signature, msg, nil)
	if err != nil {
		sp.SetTag("error", true)
		logger.Errorf("error verifying %v", err)
		return false, fmt.Errorf("error verifying: %v", err)
	}
	sp.SetTag("verified", isVerified)
	return true, nil
}
