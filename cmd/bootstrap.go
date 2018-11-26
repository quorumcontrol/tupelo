package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo/network"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/spf13/cobra"
)

var bootstrapNodePort int

var bootstrapNodeCmd = &cobra.Command{
	Use:   "bootstrap-node",
	Short: "Run a bootstrap node",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		ecdsaKeyHex := os.Getenv("NODE_ECDSA_KEY_HEX")
		ecdsaKey, err := crypto.ToECDSA(hexutil.MustDecode(ecdsaKeyHex))
		if err != nil {
			panic("error fetching ecdsa key - set env variable NODE_ECDSA_KEY_HEX")
		}

		ctx := context.Background()
		host, err := p2p.NewRelayHost(ctx, ecdsaKey, bootstrapNodePort)
		if err != nil {
			panic(fmt.Errorf("Could not start bootstrap node, %v", err))
		}

		bootstrapWithoutSelf := []string{}
		for _, nodeAddr := range network.BootstrapNodes() {
			anAddr := host.Addresses()[0].String()
			keySlice := strings.Split(anAddr, "/")
			key := keySlice[len(keySlice)-1]

			if !strings.Contains(nodeAddr, key) {
				bootstrapWithoutSelf = append(bootstrapWithoutSelf, nodeAddr)
			}
		}
		if len(bootstrapWithoutSelf) > 0 {
			host.Bootstrap(bootstrapWithoutSelf)
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
	bootstrapNodeCmd.Flags().IntVarP(&bootstrapNodePort, "port", "p", 0, "what port to use (default random)")
}
