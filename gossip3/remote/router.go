package remote

import (
	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
)

const p2pProtocol = "remoteactors/v1.0"

type router struct {
	middleware.LogAwareHolder
}

func newRouterProps() *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &router{}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (r *router) Receive(context actor.Context) {
	// defer func() {
	// 	if re := recover(); re != nil {
	// 		r.Log.Errorw("recover", "re", re)
	// 	}
	// }()
	switch msg := context.Message().(type) {
	case *registerBridge:
		incomingHandlerRegistry[msg.Peer] = msg.Handler
	case *WireDelivery:
		target := msg.Target.Address
		handler, ok := incomingHandlerRegistry[target]
		if !ok {
			r.Log.Errorw("no handler", "target", target)
			return
		}
		context.Forward(handler)
	}
}
