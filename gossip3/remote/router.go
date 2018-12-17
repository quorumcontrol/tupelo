package remote

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	pnet "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/p2p"
)

const p2pProtocol = "remoteactors/v1.0"

type router struct {
	middleware.LogAwareHolder

	bridges actorRegistry
	host    p2p.Node
}

func newRouterProps(host p2p.Node) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &router{
			host:    host,
			bridges: make(actorRegistry),
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

type internalCreateBridge struct {
	from string
	to   *ecdsa.PublicKey
}

func (r *router) Receive(context actor.Context) {
	// defer func() {
	// 	if re := recover(); re != nil {
	// 		r.Log.Errorw("recover", "re", re)
	// 	}
	// }()
	switch msg := context.Message().(type) {
	case *actor.Started:
		//TODO: what happens when the bridge dies?
		r.host.SetStreamHandler(p2pProtocol, func(s pnet.Stream) {
			context.Self().Tell(s)
		})
	case *internalCreateBridge:
		peer, err := p2p.PeerFromEcdsaKey(msg.to)
		if err != nil {
			panic(fmt.Sprintf("error getting peer from key: %v", err))
		}
		bridge, err := context.SpawnNamed(newBridgeProps(r.host, msg.to), "to-"+peer.Pretty())
		if err != nil {
			panic(fmt.Sprintf("error spawning bridge: %v", err))
		}
		r.bridges[peer.Pretty()] = bridge
	case pnet.Stream:
		remoteGateway := msg.Conn().RemotePeer().Pretty()
		handler, ok := r.bridges[remoteGateway]
		if !ok {
			r.Log.Errorw("no handler", "target", remoteGateway)
			return
		}
		context.Forward(handler)
	case *WireDelivery:
		target := routableAddress(msg.Target.Address)
		handler, ok := r.bridges[target.To()]
		if !ok {
			r.Log.Errorw("no handler", "target", target)
			return
		}
		context.Forward(handler)
	}
}
