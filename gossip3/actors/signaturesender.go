package actors

import (
	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
)

type SignatureSender struct {
	middleware.LogAwareHolder
}

// TODO: turn this into a pool
func NewSignatureSenderProps() *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return new(SignatureSender)
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (ss *SignatureSender) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.SignatureWrapper:
		for _, target := range msg.RewardsCommittee {
			ss.Log.Debugw("sending", "t", target.ID, "actor", target.Actor)
			target.Actor.Tell(msg.Signature)
		}
	}
}
