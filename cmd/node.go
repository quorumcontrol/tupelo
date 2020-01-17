package cmd

import (
	"context"
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/types"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
	"github.com/spf13/cobra"

	"github.com/quorumcontrol/tupelo/gossip3to4"
	"github.com/quorumcontrol/tupelo/nodebuilder"
)

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

		nb := &nodebuilder.NodeBuilder{Config: config}

		err = nb.Start(ctx)
		if err != nil {
			panic(fmt.Errorf("error starting: %w", err))
		}

		// spin up a gossip3to4 node
		if gossip3NotaryGroup != nil {
			err = runGossip3To4Node(ctx, gossip3NotaryGroup, nb.Actor())
			if err != nil {
				panic(err)
			}
		} else {
			fmt.Println("No gossip3 notary group configured; not starting gossip3to4 node")
		}

		fmt.Printf("Node (%s) running at:\n", nb.Host().Identity())
		for _, addr := range nb.Host().Addresses() {
			fmt.Println(addr)
		}

		<-make(chan struct{})
	},
}

func init() {
	rootCmd.AddCommand(nodeCmd)
}
