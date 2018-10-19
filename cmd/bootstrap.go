package cmd

import (
	"context"
	"fmt"
	"math/rand"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/qc3/p2p"
	"github.com/spf13/cobra"
)

var bootstrapNodeCmd = &cobra.Command{
	Use:   "bootstrap-node",
	Short: "Run a bootstrap node",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		key, err := crypto.GenerateKey()
		ctx := context.Background()
		port := rand.Intn(10000) + 30000
		host, err := p2p.NewHost(ctx, key, port)
		if err != nil {
			panic(fmt.Errorf("Could not start bootstrap node, %v", err))
		}

		fmt.Println("Bootstrap node running at:")
		for _, addr := range host.Addresses() {
			fmt.Println(addr)
		}
		select {}
	},
}

func init() {
	rootCmd.AddCommand(bootstrapNodeCmd)
}
