package cmd

import (
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/wallet/walletrpc"
	"github.com/quorumcontrol/storage"
	"github.com/spf13/cobra"
)

var rpcServerCmd = &cobra.Command{
	Use:   "rpc-server",
	Short: "Launches a Tupelo RPC Server",
	Run: func(cmd *cobra.Command, args []string) {
		memStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
		testNetGroup := consensus.NewNotaryGroup("hardcodedprivatekeysareunsafe", memStore)

		walletrpc.Serve(testNetGroup)
	},
}

func init() {
	rootCmd.AddCommand(rpcServerCmd)
}
