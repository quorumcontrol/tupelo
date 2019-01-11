package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
	gossip3client "github.com/quorumcontrol/tupelo/gossip3/client"
	gossip3remote "github.com/quorumcontrol/tupelo/gossip3/remote"
	"github.com/quorumcontrol/tupelo/wallet/walletshell"
	"github.com/spf13/cobra"
)

var shellName string

// shellCmd represents the shell command
var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Launch a Tupelo wallet shell connected to a local or remote signer network.",
	Run: func(cmd *cobra.Command, args []string) {
		gossip3remote.Start()
		if localNetworkNodeCount > 0 {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			setupLocalNetwork(ctx, localNetworkNodeCount)
		}
		walletStorage := walletPath()
		os.MkdirAll(walletStorage, 0700)
		group := setupNotaryGroup(nil, bootstrapPublicKeys)

		key, err := crypto.GenerateKey()
		if err != nil {
			panic(fmt.Sprintf("error generating key: %v", err))
		}
		group.SetupAllRemoteActors(&key.PublicKey)

		client := gossip3client.New(group)
		walletshell.RunGossip(shellName, walletStorage, client)
	},
}

func init() {
	rootCmd.AddCommand(shellCmd)
	shellCmd.Flags().StringVarP(&shellName, "name", "n", "", "the name to use for the wallet")
	shellCmd.MarkFlagRequired("name")
	shellCmd.Flags().IntVarP(&localNetworkNodeCount, "local-network", "l", 3, "Run local network with randomly generated keys, specifying number of nodes as argument. Mutually exlusive with bootstrap-*")
}
