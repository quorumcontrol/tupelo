package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	gossip3remote "github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	gossip3types "github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
	"github.com/quorumcontrol/tupelo/nodebuilder"
	"github.com/quorumcontrol/tupelo/wallet/walletshell"
	"github.com/spf13/cobra"
)

var shellName string

// shellCmd represents the shell command
var shellCmd = &cobra.Command{
	Use:   "shell",
	Short: "Launch a Tupelo wallet shell connected to a local or remote signer network.",
	Run: func(cmd *cobra.Command, args []string) {
		key, err := crypto.GenerateKey()
		if err != nil {
			panic(fmt.Errorf("error generating key: %v", err))
		}
		var group *gossip3types.NotaryGroup

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var pubSubSystem remote.PubSub
		if localNetworkNodeCount > 0 && !remoteNetwork {
			ln, err := nodebuilder.LegacyLocalNetwork(ctx, configNamespace, localNetworkNodeCount)
			if err != nil {
				panic(fmt.Errorf("error generating localnetwork: %v", err))
			}

			p2pHost, err := ln.BootstrappedP2PNode(ctx, p2p.WithKey(key))
			if err != nil {
				panic(fmt.Errorf("error creating host: %v", err))
			}
			pubSubSystem = remote.NewNetworkPubSub(p2pHost.GetPubSub())
			group = ln.NotaryGroup
		} else {
			gossip3remote.Start()

			config := nodebuilderConfig
			if config == nil {
				config, err = nodebuilder.LegacyConfig(configNamespace, 0, enableElasticTracing, enableJaegerTracing, overrideKeysFile)
				if err != nil {
					panic(fmt.Errorf("error generating legacy config: %v", err))
				}
			}

			nb := &nodebuilder.NodeBuilder{Config: config}

			p2pHost, err := nb.BootstrappedP2PNode(ctx, p2p.WithKey(key))
			if err != nil {
				panic(fmt.Errorf("error creating host: %v", err))
			}
			pubSubSystem = remote.NewNetworkPubSub(p2pHost.GetPubSub())

			gossip3remote.NewRouter(p2pHost)

			group, err = nb.NotaryGroup()
			if err != nil {
				panic(fmt.Errorf("error getting group: %v", err))
			}
		}
		group.SetupAllRemoteActors(&key.PublicKey)

		walletStorage := walletPath()
		if err = os.MkdirAll(walletStorage, 0700); err != nil {
			panic(err)
		}

		walletshell.RunGossip(shellName, walletStorage, group, pubSubSystem)
	},
}

func init() {
	rootCmd.AddCommand(shellCmd)
	shellCmd.Flags().StringVarP(&shellName, "wallet", "w", "", "the name of the wallet to access")
	if err := shellCmd.MarkFlagRequired("wallet"); err != nil {
		panic(err)
	}
}
