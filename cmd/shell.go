package cmd

import (
	"context"

	"github.com/quorumcontrol/qc3/gossip2client"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/wallet/walletshell"
	"github.com/quorumcontrol/storage"
	"github.com/spf13/cobra"
)

var shellName string

// shellCmd represents the shell command
var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Launch a Tupelo wallet shell connected to a local or remote signer network.",
	Long:  `Do not use this for anything real as it will use hard coded signing keys for the nodes`,
	Run: func(cmd *cobra.Command, args []string) {
		var bootstrapAddrs []string = network.BootstrapNodes()
		if localNetworkNodeCount > 0 {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			bootstrapAddrs = setupLocalNetwork(ctx)
		}

		group := setupNotaryGroup(storage.NewMemStorage())
		client := gossip2client.NewGossipClient(group, bootstrapAddrs)
		walletshell.RunGossip(shellName, group, client)
	},
}

func init() {
	rootCmd.AddCommand(shellCmd)
	shellCmd.Flags().StringVarP(&shellName, "name", "n", "", "the name to use for the wallet")
	shellCmd.MarkFlagRequired("name")
	shellCmd.Flags().StringVarP(&bootstrapPublicKeysFile, "bootstrap-keys", "k", "", "which keys to bootstrap the notary groups with")
	shellCmd.Flags().IntVarP(&localNetworkNodeCount, "local-network", "l", 0, "Run local network with randomly generated keys, specifying number of nodes as argument. Mutually exlusive with bootstrap-*")
}
