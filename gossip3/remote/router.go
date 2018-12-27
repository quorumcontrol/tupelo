package remote

import (
	"fmt"
	"strings"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	pnet "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	peer "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-peer"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/types"
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

// type internalCreateBridge struct {
// 	from string
// 	to   *ecdsa.PublicKey
// }

func (r *router) Receive(context actor.Context) {
	// defer func() {
	// 	if re := recover(); re != nil {
	// 		r.Log.Errorw("recover", "re", re)
	// 	}
	// }()
	switch msg := context.Message().(type) {
	case *actor.Terminated:
		split := strings.Split(msg.Who.GetId(), "/")
		to := split[len(split)-1]
		if _, ok := r.bridges[to]; ok {
			delete(r.bridges, to)
		}
	case *actor.Started:
		//TODO: what happens when the bridge dies?
		r.host.SetStreamHandler(p2pProtocol, func(s pnet.Stream) {
			context.Self().Tell(s)
		})
	case pnet.Stream:
		remoteGateway := msg.Conn().RemotePeer().Pretty()
		handler, ok := r.bridges[remoteGateway]
		if !ok {
			handler = r.createBridge(context, remoteGateway)
		}
		context.Forward(handler)
	case *WireDelivery:
		target := types.RoutableAddress(msg.Target.Address)
		handler, ok := r.bridges[target.To()]
		if !ok {
			handler = r.createBridge(context, target.To())
		}
		context.Forward(handler)
	}
}

func (r *router) createBridge(context actor.Context, to string) *actor.PID {
	p, err := peer.IDB58Decode(to)
	if err != nil {
		panic(fmt.Sprintf("error decoding pretty: %s", to))
	}
	bridge, err := context.SpawnNamed(newBridgeProps(r.host, p), p.Pretty())
	if err != nil {
		panic(fmt.Sprintf("error spawning bridge: %v", err))
	}
	r.bridges[p.Pretty()] = bridge
	return bridge
}
