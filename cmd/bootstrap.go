package cmd

import (
	"context"
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/qc3/p2p"
	"github.com/spf13/cobra"
)

const InsecureLocalPrivateKey = "0xd68934895606974010209615b8615d8c0e037a4718791f6448c3cd63c323f39c"

var bootstrapNodeCmd = &cobra.Command{
	Use:   "bootstrap-node",
	Short: "Run a bootstrap node",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		key, err := crypto.ToECDSA(hexutil.MustDecode(InsecureLocalPrivateKey))
		ctx := context.Background()
		port := 10000
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
