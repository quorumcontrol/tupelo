package actors

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/tupelo/bls"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
)

type SignatureVerifier struct {
	middleware.LogAwareHolder
}

// TODO: this should have many workers, but a single
// point of entry so that it is easy to gang up signatures to verify
func NewSignatureVerifier() *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return new(SignatureVerifier)
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (sv *SignatureVerifier) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.SignatureVerification:
		sv.handleSignatureVerification(context, msg)
	}
}

func (sv *SignatureVerifier) handleSignatureVerification(context actor.Context, msg *messages.SignatureVerification) {
	isVerified, err := bls.VerifyMultiSig(msg.Signature, msg.Message, msg.VerKeys)
	if err != nil {
		sv.Log.Errorw("error verifying", "err", err)
		panic(fmt.Sprintf("error verifying: %v", err))
	}
	msg.Verified = isVerified
	context.Respond(msg)
}
