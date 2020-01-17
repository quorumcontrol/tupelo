package cmd

import (
	"context"
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/types"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
	"github.com/quorumcontrol/tupelo-go-sdk/tracing"
	"github.com/spf13/cobra"

	"github.com/quorumcontrol/tupelo/gossip"
	"github.com/quorumcontrol/tupelo/gossip3to4"
	"github.com/quorumcontrol/tupelo/nodebuilder"
)

func runGossipNode(ctx context.Context, config *nodebuilder.Config, group *types.NotaryGroup) (*actor.PID, error) {
	p2pNode, bitswapper, err := p2p.NewHostAndBitSwapPeer(
		ctx,
		p2p.WithKey(config.PrivateKeySet.DestKey),
		// TODO: this is easier for early development of wasm, but we should examine whether
		// we want this in production or not.
		p2p.WithWebSockets(50000),
	)
	if err != nil {
		return nil, fmt.Errorf("error creating p2p node: %v", err)
	}

	nodeCfg := &gossip.NewNodeOptions{
		P2PNode:     p2pNode,
		SignKey:     config.PrivateKeySet.SignKey,
		NotaryGroup: group,
		DagStore:    bitswapper,
	}

	node, err := gossip.NewNode(ctx, nodeCfg)
	if err != nil {
		return nil, fmt.Errorf("error creating new node: %v", err)
	}

	err = node.Bootstrap(ctx, group.Config().BootstrapAddresses)
	if err != nil {
		return nil, fmt.Errorf("error bootstrapping node: %v", err)
	}

	fmt.Printf("node bootstrapped, starting")

	err = node.Start(ctx)
	if err != nil {
		return nil, fmt.Errorf("error starting node: %v", err)
	}

	fmt.Printf("started signer host %s on %v\n", p2pNode.Identity(), p2pNode.Addresses())

	return node.PID(), nil
}

func runGossip3To4Node(ctx context.Context, group *types.NotaryGroup, gossipPID *actor.PID) error {
	p2pNode, _, err := p2p.NewHostAndBitSwapPeer(ctx)
	if err != nil {
		return fmt.Errorf("error creating p2p node: %v", err)
	}

	node := gossip3to4.NewNode(ctx, &gossip3to4.NodeConfig{
		P2PNode:     p2pNode,
		NotaryGroup: group,
		Gossip4Node: gossipPID,
	})

	node.Start(ctx)

	return node.Bootstrap(ctx, group.Config().BootstrapAddresses)
}

var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Run a tupelo node (gossip)",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		config := nodebuilderConfig
		if config == nil {
			panic(fmt.Errorf("error getting node config"))
		}

		// start the tracing system if configured
		switch config.TracingSystem {
		case nodebuilder.ElasticTracing:
			fmt.Println("Starting elastic tracing")
			tracing.StartElastic()
		case nodebuilder.NoTracing:
			// no-op
		default:
			panic(fmt.Errorf("only elastic tracing is supported; got %v", config.TracingSystem))
		}

		// get the gossip3 notary group
		var (
			gossip3NotaryGroup *types.NotaryGroup
			err                error
		)
		if config.Gossip3NotaryGroupConfig != nil {
			gossip3NotaryGroup, err = config.NotaryGroupConfig.NotaryGroup(nil)
			if err != nil {
				panic(fmt.Errorf("error generating notary group: %v", err))
			}
		}

		// get the gossip4 notary group
		localKeys := config.PrivateKeySet
		localSigner := types.NewLocalSigner(&localKeys.DestKey.PublicKey, localKeys.SignKey)
		gossipNotaryGroup, err := config.NotaryGroupConfig.NotaryGroup(localSigner)
		if err != nil {
			panic(fmt.Errorf("error generating notary group: %v", err))
		}

		// spin up a gossip4 node
		pid, err := runGossipNode(ctx, config, gossipNotaryGroup)
		if err != nil {
			panic(err)
		}

		// spin up a gossip3to4 node
		if gossip3NotaryGroup != nil {
			err = runGossip3To4Node(ctx, gossip3NotaryGroup, pid)
			if err != nil {
				panic(err)
			}
		} else {
			fmt.Println("No gossip3 notary group configured; not starting gossip3to4 node")
		}

		fmt.Println("Node running")

		<-make(chan struct{})
	},
}

func init() {
	rootCmd.AddCommand(nodeCmd)
}
