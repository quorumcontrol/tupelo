package gossip3to4

import (
	"fmt"
	"reflect"

	"github.com/AsynkronIT/protoactor-go/actor"
	logging "github.com/ipfs/go-log"
	"github.com/opentracing/opentracing-go"
	"github.com/quorumcontrol/messages/build/go/services"
	g4services "github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	g3types "github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
)

const SubscriptionSuffix = "-g3-to-g4-translator"

type Gossip3SubscriberConfig struct {
	P2PNode     p2p.Node
	NotaryGroup *g3types.NotaryGroup
}

type Gossip3Subscriber struct {
	p2pNode     p2p.Node
	notaryGroup *g3types.NotaryGroup
	pid         *actor.PID
	logger      logging.EventLogger
}

func NewGossip3SubscriberProps(cfg *Gossip3SubscriberConfig) *actor.Props {
	logger := logging.Logger("gossip3to4-gossip3-sub")

	return actor.PropsFromProducer(func() actor.Actor {
		return &Gossip3Subscriber{
			p2pNode:     cfg.P2PNode,
			notaryGroup: cfg.NotaryGroup,
			logger:      logger,
		}
	})
}

func (g3s *Gossip3Subscriber) Receive(actorCtx actor.Context) {
	switch msg := actorCtx.Message().(type) {
	case *actor.Started:
		g3s.handleStarted(actorCtx)
	case *services.AddBlockRequest:
		g3s.logger.Debugf("received ABR: %+v", msg)
		g3s.handleAddBlockRequest(actorCtx, msg)
	case *g4services.AddBlockRequest:
		g3s.logger.Debugf("received g4 (%T) ABR: %+v", msg, msg)
		sp := opentracing.StartSpan("gossip3to4-g3sub-received-g4abr")
		sp.SetTag("abr", msg)
		sp.SetTag("abr_type", reflect.TypeOf(msg))
		defer sp.Finish()
	default:
		g3s.logger.Debugf("received other message (%T): %+v", msg, msg)
		sp := opentracing.StartSpan("gossip3to4-g3sub-received-other")
		sp.SetTag("message", msg)
		sp.SetTag("message_type", reflect.TypeOf(msg))
		defer sp.Finish()
	}
}

func (g3s *Gossip3Subscriber) handleStarted(actorCtx actor.Context) {
	sp := opentracing.StartSpan("gossip3to4-gossip3-transaction-subscriber-started")
	defer sp.Finish()

	pubsub := remote.NewNetworkPubSub(g3s.p2pNode.GetPubSub())

	ngCfg := g3s.notaryGroup.Config()

	pid, err := actorCtx.SpawnNamed(pubsub.NewSubscriberProps(ngCfg.TransactionTopic), ngCfg.ID+SubscriptionSuffix)
	if err != nil {
		panic(fmt.Errorf("error spawning gossip3 subscriber actor: %v", err))
	}

	g3s.pid = pid
}

func (g3s *Gossip3Subscriber) handleAddBlockRequest(actorCtx actor.Context, abr *services.AddBlockRequest) {
	sp := opentracing.StartSpan("add-block-request-gossip3-subscriber")
	defer sp.Finish()

	g4ABR, err := ConvertABR(abr)
	if err != nil {
		panic(fmt.Errorf("error converting gossip3 ABR to gossip4 format: %v", err))
	}

	actorCtx.Send(actorCtx.Parent(), g4ABR)
}
