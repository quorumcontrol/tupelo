package cmd

import (
	"context"
	"fmt"

	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
	"github.com/spf13/cobra"

	"github.com/quorumcontrol/tupelo/gossip4"
)

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

		// spin up a gossip4 node
		p2pNode, peer, err := p2p.NewHostAndBitSwapPeer(ctx)
		if err != nil {
			panic(fmt.Errorf("error creating p2p node: %v", err))
		}

		localSigner := types.NewLocalSigner(&config.PrivateKeySet.DestKey.PublicKey, config.PrivateKeySet.SignKey)
		group, err := config.NotaryGroupConfig.NotaryGroup(localSigner)
		if err != nil {
			panic(fmt.Errorf("error generating notary group: %v", err))
		}

		node, err := gossip4.NewNode(ctx, &gossip4.NewNodeOptions{
			P2PNode:     p2pNode,
			SignKey:     config.PrivateKeySet.SignKey,
			NotaryGroup: group,
			DagStore:    peer,
		})
		if err != nil {
			panic(fmt.Errorf("error creating new node: %v", err))
		}

		err = node.Start(ctx)
		if err != nil {
			panic(fmt.Errorf("error starting node: %v", err))
		}

		fmt.Println("Node running")

		<- make(chan struct{})

		// spin up a gossip3 subscriber (not a full node)

		// forward messages from gossip3 subscriber to a gossip4 conversion func

		// forward converted gossip3 messages to gossip4 topic
	},
}

func init() {
	rootCmd.AddCommand(nodeCmd)
}
