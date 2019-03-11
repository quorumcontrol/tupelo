package cmd

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
	gossip3client "github.com/quorumcontrol/tupelo-go-client/client"
	gossip3remote "github.com/quorumcontrol/tupelo-go-client/gossip3/remote"
	gossip3types "github.com/quorumcontrol/tupelo-go-client/gossip3/types"
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
		if localNetworkNodeCount > 0 && !remoteNetwork {
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
		if err = os.MkdirAll(walletStorage, 0700); err != nil {
			panic(err)
		}

		client := gossip3client.New(group)
		walletshell.RunGossip(shellName, walletStorage, client)
	},
}

func init() {
	rootCmd.AddCommand(shellCmd)
	shellCmd.Flags().StringVarP(&shellName, "wallet", "w", "", "the name of the wallet to access")
	if err := shellCmd.MarkFlagRequired("wallet"); err != nil {
		panic(err)
	}
}
