package cmd

import (
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
		if localNetworkNodeCount > 0 {
			setupLocalNetwork()
		}

		group := setupNotaryGroup(storage.NewMemStorage())
		walletshell.RunGossip(shellName, group)
	},
}

func init() {
	rootCmd.AddCommand(shellCmd)
	shellCmd.Flags().StringVarP(&shellName, "name", "n", "alice", "the name to use for the wallet")
	shellCmd.Flags().StringVarP(&bootstrapPublicKeysFile, "bootstrap-keys", "k", "", "which keys to bootstrap the notary groups with")
	shellCmd.Flags().IntVarP(&localNetworkNodeCount, "local-network", "l", 0, "Run local network with randomly generated keys, specifying number of nodes as argument. Mutually exlusive with bootstrap-*")

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// shellCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// shellCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
