package gossip3to4

import (
	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/opentracing/opentracing-go"
	"github.com/quorumcontrol/messages/build/go/services"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
)

type NodeConfig struct {
	P2PNode     p2p.Node
	NotaryGroup *types.NotaryGroup
	Gossip4Node *actor.PID
}

type Node struct {
	p2pNode     p2p.Node
	notaryGroup *types.NotaryGroup
	gossip3Sub  *actor.PID
	gossip4Node *actor.PID
}

func NewNodeProps(cfg *NodeConfig) *actor.Props {
	return actor.PropsFromProducer(func() actor.Actor {
		return &Node{
			p2pNode:     cfg.P2PNode,
			notaryGroup: cfg.NotaryGroup,
			gossip4Node: cfg.Gossip4Node,
		}
	}).WithReceiverMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (n *Node) Receive(actorCtx actor.Context) {
	switch actorCtx.Message().(type) {
	case *actor.Started:
		n.handleStarted(actorCtx)
	case *services.AddBlockRequest:
		// these are converted ABRs coming back from the gossip3 subscriber
		n.handleAddBlockRequest(actorCtx)
	}
}

func (n *Node) handleStarted(actorCtx actor.Context) {
	g3sCfg := &Gossip3SubscriberConfig{
		P2PNode:     n.p2pNode,
		NotaryGroup: n.notaryGroup,
	}
	n.gossip3Sub = actorCtx.Spawn(NewGossip3SubscriberProps(g3sCfg))
}

func (n *Node) handleAddBlockRequest(actorCtx actor.Context) {
	sp := opentracing.StartSpan("add-block-request-gossip3-to-gossip4")
	defer sp.Finish()

	actorCtx.Forward(n.gossip4Node)
}
