package cmd

import (
	"fmt"
	"time"

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
		if testNetGroup.IsGenesis() {
			testNetMembers := bootstrapMembers(bootstrapKeysFile)
			fmt.Printf("Bootstrapping notary group with %v nodes\n", len(testNetMembers))
			testNetGroup.CreateGenesisState(testNetGroup.RoundAt(time.Now()), testNetMembers...)
		}

		walletrpc.Serve(testNetGroup)
	},
}

func init() {
	rootCmd.AddCommand(rpcServerCmd)
	rpcServerCmd.Flags().StringVarP(&bootstrapKeysFile, "bootstrap-keys", "k", "", "which keys to bootstrap the notary groups with")
}
