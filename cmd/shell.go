package cmd

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
	gossip3client "github.com/quorumcontrol/tupelo/gossip3/client"
	gossip3remote "github.com/quorumcontrol/tupelo/gossip3/remote"
	gossip3types "github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/wallet/walletshell"
	"github.com/spf13/cobra"
)

var shellName string

// shellCmd represents the shell command
var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Launch a Tupelo wallet shell connected to a local or remote signer network.",
	Run: func(cmd *cobra.Command, args []string) {
		var key *ecdsa.PrivateKey
		var err error
		var group *gossip3types.NotaryGroup
		if localNetworkNodeCount > 0 {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			group = setupLocalNetwork(ctx, localNetworkNodeCount)
		} else {
			key, err = crypto.GenerateKey()
			if err != nil {
				panic(fmt.Sprintf("error generating key: %v", err))
			}
			gossip3remote.Start()
			group = setupNotaryGroup(nil, bootstrapPublicKeys)
			group.SetupAllRemoteActors(&key.PublicKey)
		}
		walletStorage := walletPath()
		os.MkdirAll(walletStorage, 0700)

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
