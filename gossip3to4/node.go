package gossip3to4

import (
	"context"
	"fmt"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	logging "github.com/ipfs/go-log"
	"github.com/opentracing/opentracing-go"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/types"
	g3types "github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
	"github.com/quorumcontrol/tupelo/gossip"
)

type NodeConfig struct {
	P2PNode            p2p.Node
	NotaryGroup        *types.NotaryGroup
	Gossip3NotaryGroup *g3types.NotaryGroup
	Gossip4Node        *actor.PID
}

type Node struct {
	p2pNode            p2p.Node
	notaryGroup        *types.NotaryGroup
	gossip3NotaryGroup *g3types.NotaryGroup
	gossip3Sub         *actor.PID
	gossip4Node        *actor.PID
	validator          *gossip.TransactionValidator
	logger             logging.EventLogger
}

func NewNode(ctx context.Context, cfg *NodeConfig) *Node {
	logger := logging.Logger("gossip3to4")

	// purposely setting the node PID to nil because we send in the ABR
	// from this node as opposed to using the validator itself to send in the Tx
	validator, err := gossip.NewTransactionValidator(ctx, logger, cfg.NotaryGroup, nil)
	if err != nil {
		panic(fmt.Errorf("error creating new transaction validator: %w", err))
	}

	return &Node{
		p2pNode:            cfg.P2PNode,
		notaryGroup:        cfg.NotaryGroup,
		gossip3NotaryGroup: cfg.Gossip3NotaryGroup,
		gossip4Node:        cfg.Gossip4Node,
		validator:          validator,
		logger:             logger,
	}
}

func (n *Node) Start(ctx context.Context) {
	pid := actor.EmptyRootContext.Spawn(actor.PropsFromFunc(n.Receive))
	go func() {
		<-ctx.Done()
		n.logger.Debugf("node stopped")
		actor.EmptyRootContext.Poison(pid)
	}()

	n.logger.Debug("node started")
}

func (n *Node) Bootstrap(ctx context.Context, bootstrapAddrs []string) error {
	n.logger.Debug("p2p node bootstrapping")

	_, err := n.p2pNode.Bootstrap(bootstrapAddrs)
	if err != nil {
		return fmt.Errorf("error bootstrapping gosssip3to4 node: %v", err)
	}

	return n.p2pNode.WaitForBootstrap(1, 5*time.Second)
}

func (n *Node) Receive(actorCtx actor.Context) {
	switch msg := actorCtx.Message().(type) {
	case *actor.Started:
		n.handleStarted(actorCtx)
	case *services.AddBlockRequest:
		// these are converted ABRs coming back from the gossip3 subscriber
		n.logger.Debugf("received ABR: %+v", msg)
		n.handleAddBlockRequest(actorCtx, msg)
	default:
		n.logger.Debugf("received other message: %+v", msg)
		sp := opentracing.StartSpan("gossip3to4-node-received-other")
		sp.SetTag("message", msg)
		defer sp.Finish()
	}
}

func (n *Node) handleStarted(actorCtx actor.Context) {
	sp := opentracing.StartSpan("gossip3to4-node-started")
	defer sp.Finish()

	g3sCfg := &Gossip3SubscriberConfig{
		P2PNode:     n.p2pNode,
		NotaryGroup: n.gossip3NotaryGroup,
	}
	n.gossip3Sub = actorCtx.Spawn(NewGossip3SubscriberProps(g3sCfg))
}

func (n *Node) handleAddBlockRequest(actorCtx actor.Context, abr *services.AddBlockRequest) {
	sp := opentracing.StartSpan("gossip3to4-received-add-block-request")
	defer sp.Finish()

	valid := n.validator.ValidateAbr(context.TODO(), abr)
	if valid {
		sp.SetTag("valid", true)
		wrapper := &gossip.AddBlockWrapper{
			AddBlockRequest: abr,
		}
		wrapper.StartTrace("gossip3.transaction")

		actorCtx.Send(n.gossip4Node, wrapper)

		return
	}
	sp.SetTag("valid", false)
}
