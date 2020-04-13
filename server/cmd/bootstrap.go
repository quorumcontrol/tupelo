package cmd

import (
	"context"
	"fmt"

	"github.com/quorumcontrol/tupelo/server/nodebuilder"

	"github.com/spf13/cobra"
)

var bootstrapNodePort int

var bootstrapNodeCmd = &cobra.Command{
	Use:   "bootstrap-node",
	Short: "Run a bootstrap node",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		c := nodebuilderConfig
		if c == nil {
			panic("you must specify a bootstrap config")
		}

		nb := &nodebuilder.NodeBuilder{Config: c}

		err := nb.Start(ctx)
		if err != nil {
			panic(fmt.Errorf("error starting bootstrap: %v", err))
		}

		fmt.Println("Bootstrap node running at:")
		for _, addr := range nb.Host().Addresses() {
			fmt.Println(addr)
		}
		select {}
	},
}

func init() {
	rootCmd.AddCommand(bootstrapNodeCmd)
	bootstrapNodeCmd.Flags().IntVarP(&bootstrapNodePort, "port", "p", 0, "what port to use (default random)")
}
