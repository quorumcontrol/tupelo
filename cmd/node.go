package cmd

import (
	"context"
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	g3types "github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip4/types"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
	"github.com/quorumcontrol/tupelo-go-sdk/tracing"
	"github.com/spf13/cobra"

	"github.com/quorumcontrol/tupelo/gossip3to4"
	"github.com/quorumcontrol/tupelo/gossip4"
	"github.com/quorumcontrol/tupelo/nodebuilder"
)

func runGossip4Node(ctx context.Context, config *nodebuilder.Config, group *types.NotaryGroup) (*actor.PID, error) {
	p2pNode, peer, err := p2p.NewHostAndBitSwapPeer(ctx)
	if err != nil {
		return nil, fmt.Errorf("error creating p2p node: %v", err)
	}

	nodeCfg := &gossip4.NewNodeOptions{
		P2PNode:     p2pNode,
		SignKey:     config.PrivateKeySet.SignKey,
		NotaryGroup: group,
		DagStore:    peer,
	}

	node, err := gossip4.NewNode(ctx, nodeCfg)
	if err != nil {
		return nil, fmt.Errorf("error creating new node: %v", err)
	}

	err = node.Start(ctx)
	if err != nil {
		return nil, fmt.Errorf("error starting node: %v", err)
	}

	err = node.Bootstrap(ctx, group.Config().BootstrapAddresses)
	if err != nil {
		return nil, fmt.Errorf("error bootstrapping node: %v", err)
	}

	return node.PID(), nil
}

func runGossip3To4Node(ctx context.Context, group *g3types.NotaryGroup, gossip4PID *actor.PID) error {
	p2pNode, _, err := p2p.NewHostAndBitSwapPeer(ctx)
	if err != nil {
		return fmt.Errorf("error creating p2p node: %v", err)
	}

	node := gossip3to4.NewNode(ctx, &gossip3to4.NodeConfig{
		P2PNode:     p2pNode,
		NotaryGroup: group,
		Gossip4Node: gossip4PID,
	})

	node.Start(ctx)

	return node.Bootstrap(ctx, group.Config().BootstrapAddresses)
}

var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Run a tupelo node (gossip4)",
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
			gossip3NotaryGroup *g3types.NotaryGroup
			err                error
		)
		if config.Gossip3NotaryGroupConfig != nil {
			gossip3NotaryGroup, err = config.Gossip3NotaryGroupConfig.NotaryGroup(nil)
			if err != nil {
				panic(fmt.Errorf("error generating notary group: %v", err))
			}
		}

		// get the gossip4 notary group
		localKeys := config.PrivateKeySet
		localSigner := types.NewLocalSigner(&localKeys.DestKey.PublicKey, localKeys.SignKey)
		gossip4NotaryGroup, err := config.NotaryGroupConfig.NotaryGroup(localSigner)
		if err != nil {
			panic(fmt.Errorf("error generating notary group: %v", err))
		}

		// spin up a gossip4 node
		pid, err := runGossip4Node(ctx, config, gossip4NotaryGroup)
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
