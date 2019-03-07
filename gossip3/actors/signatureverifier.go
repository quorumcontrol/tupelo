package actors

import (
	"fmt"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/AsynkronIT/protoactor-go/router"
	"github.com/quorumcontrol/tupelo/bls"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
)

type SignatureVerifier struct {
	middleware.LogAwareHolder
	verifierFarm *actor.PID
	currentBatch []*verificationRequest
}

type verificationRequest struct {
	message *messages.SignatureVerification
	sender  *actor.PID
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
	switch msg := context.Message().(type) {
	case *actor.Started:
		sv.verifierFarm = context.Spawn(router.NewRoundRobinPool(verifierConcurrency).WithFunc(sv.handleSignatureVerification))
	case *actor.ReceiveTimeout:
		context.SetReceiveTimeout(0)
		context.Request(sv.verifierFarm, sv.currentBatch)
		sv.currentBatch = nil
	case *verificationRequest:
		// if it came back to this level, it means it needs to be individually verified
		sv.verifierFarm.Request(msg.message, msg.sender)
	case *messages.SignatureVerification:
		verRequest := &verificationRequest{
			message: msg,
			sender:  context.Sender(),
		}
		sv.currentBatch = append(sv.currentBatch, verRequest)
		if len(sv.currentBatch) >= 50 {
			context.SetReceiveTimeout(0)
			context.Request(sv.verifierFarm, sv.currentBatch)
			sv.currentBatch = nil
		} else {
			context.SetReceiveTimeout(10 * time.Millisecond)
		}
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
	case []*verificationRequest:
		sv.Log.Debugw("batch received", "len", len(msg))
		msgs := make([][]byte, len(msg))
		keys := make([]*bls.VerKey, len(msg))
		sigs := make([][]byte, len(msg))
		var err error
		for i, verRequest := range msg {
			msgs[i] = verRequest.message.Message
			key, intErr := bls.SumPublics(verRequest.message.VerKeys)
			if intErr != nil {
				err = intErr
				break
			}
			keys[i] = key
			sigs[i] = verRequest.message.Signature
		}
		if err != nil {
			sv.Log.Warnw("error summing publics", "err", err)
			doIndividualVerification(context, msg)
			return
		}
		sv.Log.Debugw("ready to batch", "msgs", msgs)

		isValid, err := bls.BatchVerify(msgs, keys, sigs)
		if !isValid || err != nil {
			sv.Log.Warnw("error batch verifying", "err", err)
			doIndividualVerification(context, msg)
			return
		}
		// otherwise, everything in the batch was good!
		for _, verRequest := range msg {
			sigVer := verRequest.message
			sigVer.Verified = true
			verRequest.sender.Tell(sigVer)
		}
	}
}

func doIndividualVerification(context actor.Context, msg []*verificationRequest) {
	for _, verRequest := range msg {
		context.Respond(verRequest)
	}
}
