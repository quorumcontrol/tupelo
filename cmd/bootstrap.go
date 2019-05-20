package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/libp2p/go-libp2p"
	circuit "github.com/libp2p/go-libp2p-circuit"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
	"github.com/spf13/cobra"
)

var bootstrapNodePort int

var bootstrapNodeCmd = &cobra.Command{
	Use:   "bootstrap-node",
	Short: "Run a bootstrap node",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		ecdsaKeyHex := os.Getenv("TUPELO_NODE_ECDSA_KEY_HEX")
		ecdsaKey, err := crypto.ToECDSA(hexutil.MustDecode(ecdsaKeyHex))
		if err != nil {
			panic(fmt.Sprintf("error decoding ecdsa key: %s", err))
		}

		ctx := context.Background()

		cm := connmgr.NewConnManager(4915, 7372, 30*time.Second)

		host, err := p2pNodeWithOpts(
			ctx,
			ecdsaKey,
			bootstrapNodePort,
			p2p.WithLibp2pOptions(libp2p.ConnectionManager(cm)),
			p2p.WithRelayOpts(circuit.OptHop),
		)
		if err != nil {
			panic(fmt.Errorf("Could not start bootstrap node, %v", err))
		}

		anAddr := host.Addresses()[0].String()
		keySlice := strings.Split(anAddr, "/")
		key := keySlice[len(keySlice)-1]

		bootstrapWithoutSelf := []string{}
		for _, nodeAddr := range p2p.BootstrapNodes() {
			if !strings.Contains(nodeAddr, key) {
				bootstrapWithoutSelf = append(bootstrapWithoutSelf, nodeAddr)
			}
		}
		if len(bootstrapWithoutSelf) > 0 {
			if _, err = host.Bootstrap(bootstrapWithoutSelf); err != nil {
				panic(fmt.Sprintf("bootstrapping failed: %s", err))
			}
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
