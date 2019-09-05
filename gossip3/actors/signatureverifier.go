package actors

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/AsynkronIT/protoactor-go/router"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/middleware"
	sigfuncs "github.com/quorumcontrol/tupelo-go-sdk/signatures"
	"github.com/quorumcontrol/tupelo-go-sdk/tracing"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
)

type SignatureVerifier struct {
	middleware.LogAwareHolder
	verifierFarm *actor.PID
}

func NewSignatureVerifier() *actor.Props {
	return actor.PropsFromProducer(func() actor.Actor {
		return new(SignatureVerifier)
	}).WithReceiverMiddleware(
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
		var sp opentracing.Span
		if traceable, ok := msg.Memo.(tracing.Traceable); ok {
			sp = traceable.NewSpan("signatureVerification")
		} else {
			sp = opentracing.StartSpan("signatureVerification")
		}
		defer sp.Finish()

		sv.Log.Debugw("handle signature verification")

		err := sigfuncs.RestoreBLSPublicKey(msg.Signature, msg.VerKeys)
		if err != nil {
			sp.SetTag("error", true)
			sv.Log.Errorw("error restoring public key", "err", err)
			panic(fmt.Sprintf("error verifying: %v", err)) // TODO: do we need to panic here?
		}

		isVerified, err := sigfuncs.Valid(msg.Signature, msg.Message, nil)
		if err != nil {
			sp.SetTag("error", true)
			sv.Log.Errorw("error verifying", "err", err)
			panic(fmt.Sprintf("error verifying: %v", err))
		}
		sp.SetTag("verified", isVerified)
		msg.Verified = isVerified
		context.Respond(msg)
	}

}
