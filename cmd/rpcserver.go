package cmd

import (
	"github.com/quorumcontrol/qc3/wallet/walletrpc"
	"github.com/spf13/cobra"
)

var rpcServerCmd = &cobra.Command{
	Use:   "rpc-server",
	Short: "Launches a Tupelo RPC Server",
	Run: func(cmd *cobra.Command, args []string) {
		walletrpc.Serve(TestNetGroup)
	},
}

func init() {
	rootCmd.AddCommand(rpcServerCmd)
}
