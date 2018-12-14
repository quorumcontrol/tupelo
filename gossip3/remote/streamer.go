package remote

import (
	"log"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	pnet "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/p2p"
)

type streamHolder map[string]pnet.Stream

type streamer struct {
	middleware.LogAwareHolder
	streams streamHolder
	host    p2p.Node
}

func newStreamerProps(host p2p.Node) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &streamer{
			streams: make(streamHolder),
			host:    host,
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (s *streamer) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *WireDelivery:
		log.Printf("wire delivery: %v", msg)
	}
}
