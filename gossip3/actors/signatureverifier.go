package actors

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/AsynkronIT/protoactor-go/router"
	"github.com/quorumcontrol/tupelo-go-client/bls"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
)

type SignatureVerifier struct {
	middleware.LogAwareHolder
	verifierFarm *actor.PID
}

func NewSignatureVerifier() *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return new(SignatureVerifier)
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

const verifierConcurrency = 20

// this is a singleton that farms out to a farm so that in the future
// we can gang up these verifications
func (sv *SignatureVerifier) Receive(context actor.Context) {
	switch context.Message().(type) {
	case *actor.Started:
		sv.verifierFarm = context.Spawn(router.NewRoundRobinPool(verifierConcurrency).WithFunc(sv.handleSignatureVerification))
	case *messages.SignatureVerification:
		context.Forward(sv.verifierFarm)
	}
}

func (sv *SignatureVerifier) handleSignatureVerification(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.SignatureVerification:
		sv.Log.Debugw("handle signature verification")
		isVerified, err := bls.VerifyMultiSig(msg.Signature, msg.Message, msg.VerKeys)
		if err != nil {
			sv.Log.Errorw("error verifying", "err", err)
			panic(fmt.Sprintf("error verifying: %v", err))
		}
		msg.Verified = isVerified
		context.Respond(msg)
	}

}
