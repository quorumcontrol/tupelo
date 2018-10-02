package cmd

import (
	"fmt"
	"time"

	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/wallet/walletshell"
	"github.com/quorumcontrol/storage"
	"github.com/spf13/cobra"
)

var shellName string

// shellCmd represents the shell command
var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "launches a shell wallet that can only talk to the testnet",
	Long:  `Do not use this for anything real as it will use hard coded signing keys for the nodes`,
	Run: func(cmd *cobra.Command, args []string) {
		nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
		group := consensus.NewNotaryGroup("hardcodedprivatekeysareunsafe", nodeStore)
		if group.IsGenesis() {
			testNetMembers := bootstrapMembers(bootstrapPublicKeysFile)
			fmt.Printf("Bootstrapping notary group with %v nodes\n", len(testNetMembers))
			group.CreateGenesisState(group.RoundAt(time.Now()), testNetMembers...)
		}
		walletshell.RunGossip(shellName, group)
	},
}

func init() {
	rootCmd.AddCommand(shellCmd)
	shellCmd.Flags().StringVarP(&shellName, "name", "n", "alice", "the name to use for the wallet")
	shellCmd.Flags().StringVarP(&bootstrapPublicKeysFile, "bootstrap-keys", "k", "", "which keys to bootstrap the notary groups with")

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// shellCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// shellCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
