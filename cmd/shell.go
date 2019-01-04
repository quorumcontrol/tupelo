package cmd

import (
	"context"

	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/quorumcontrol/tupelo/wallet/walletshell"
	"github.com/spf13/cobra"
)

var shellName string

// shellCmd represents the shell command
var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Launch a Tupelo wallet shell connected to a local or remote signer network.",
	Run: func(cmd *cobra.Command, args []string) {
		var bootstrapAddrs []string
		if localNetworkNodeCount > 0 {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			bootstrapAddrs = setupLocalNetwork(ctx, localNetworkNodeCount)
		} else {
			bootstrapAddrs = p2p.BootstrapNodes()
		}

		walletStorage := walletPath()
		client := startClient(bootstrapAddrs)
		walletshell.RunGossip(shellName, walletStorage, client)
	},
}

func init() {
	rootCmd.AddCommand(shellCmd)
	shellCmd.Flags().StringVarP(&shellName, "name", "n", "", "the name to use for the wallet")
	shellCmd.MarkFlagRequired("name")
	shellCmd.Flags().IntVarP(&localNetworkNodeCount, "local-network", "l", 3, "Run local network with randomly generated keys, specifying number of nodes as argument. Mutually exlusive with bootstrap-*")
}
