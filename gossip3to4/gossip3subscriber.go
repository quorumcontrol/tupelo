package gossip3to4

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/opentracing/opentracing-go"
	g3services "github.com/quorumcontrol/messages/build/go/services"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
)

const SubscriptionSuffix = "-g3-to-g4-translator"

type Gossip3SubscriberConfig struct {
	P2PNode     p2p.Node
	NotaryGroup *types.NotaryGroup
}

type Gossip3Subscriber struct {
	p2pNode     p2p.Node
	notaryGroup *types.NotaryGroup
	pid         *actor.PID
}

func NewGossip3SubscriberProps(cfg *Gossip3SubscriberConfig) *actor.Props {
	return actor.PropsFromProducer(func() actor.Actor {
		return &Gossip3Subscriber{
			p2pNode:     cfg.P2PNode,
			notaryGroup: cfg.NotaryGroup,
		}
	})
}

func (g3s *Gossip3Subscriber) Receive(actorCtx actor.Context) {
	switch msg := actorCtx.Message().(type) {
	case *actor.Started:
		g3s.handleStarted(actorCtx)
	case *g3services.AddBlockRequest:
		g3s.handleAddBlockRequest(actorCtx, msg)
	}
}

func (g3s *Gossip3Subscriber) handleStarted(actorCtx actor.Context) {
	pubsub := remote.NewNetworkPubSub(g3s.p2pNode.GetPubSub())

	ngCfg := g3s.notaryGroup.Config()

	pid, err := actorCtx.SpawnNamed(pubsub.NewSubscriberProps(ngCfg.TransactionTopic), ngCfg.ID + SubscriptionSuffix)
	if err != nil {
		panic(fmt.Errorf("error spawning gossip3 subscriber actor: %v", err))
	}

	g3s.pid = pid
}

func (g3s *Gossip3Subscriber) handleAddBlockRequest(actorCtx actor.Context, abr *g3services.AddBlockRequest) {
	sp := opentracing.StartSpan("add-block-request-gossip3-subscriber")
	defer sp.Finish()

	g4ABR, err := ConvertABR(abr)
	if err != nil {
		panic(fmt.Errorf("error converting gossip3 ABR to gossip4 format: %v", err))
	}

	actorCtx.Respond(g4ABR)
}
