package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/quorumcontrol/tupelo/signer/nodebuilder"
)

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

		nb := &nodebuilder.NodeBuilder{Config: config}

		err := nb.Start(ctx)
		if err != nil {
			panic(fmt.Errorf("error starting: %w", err))
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
