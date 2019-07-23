package cmd

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"strings"

	"github.com/quorumcontrol/tupelo/nodebuilder"

	"google.golang.org/grpc"

	"github.com/quorumcontrol/tupelo/wallet/walletrpc"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	gossip3remote "github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	gossip3types "github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
	"github.com/spf13/cobra"
)

const defaultPort = 50051

func panicWithoutTLSOpts() {
	if certFile == "" || keyFile == "" {
		var msg strings.Builder
		if certFile == "" {
			msg.WriteString("Missing certificate file path. ")
			msg.WriteString("Please supply with the -C flag. ")
		}

		if keyFile == "" {
			msg.WriteString("Missing key file path. ")
			msg.WriteString("Please supply with the -K flag. ")
		}

		panic(msg.String())
	}
}

func walletPath() string {
	return configDir("wallets")
}

var (
	certFile              string
	keyFile               string
	localNetworkNodeCount int
	tls                   bool
)

var rpcServerCmd = &cobra.Command{
	Use:   "rpc-server",
	Short: "Launches a Tupelo RPC Server",
	Run: func(cmd *cobra.Command, args []string) {
		var key *ecdsa.PrivateKey
		var err error
		var group *gossip3types.NotaryGroup
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		var pubSubSystem remote.PubSub
		key, err = crypto.GenerateKey()
		if err != nil {
			panic(fmt.Errorf("error generating key: %v", err))
		}

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

		var grpcServer *grpc.Server

		if tls {
			panicWithoutTLSOpts()
			server, err := walletrpc.ServeTLS(walletStorage, group, pubSubSystem, certFile, keyFile, defaultPort)
			if err != nil {
				panic(err)
			}
			grpcServer = server
		} else {
			server, err := walletrpc.ServeInsecure(walletStorage, group, pubSubSystem, defaultPort)
			if err != nil {
				panic(err)
			}
			grpcServer = server
		}

		if rpcServeWebGrpc {
			if tls {
				_, err := walletrpc.ServeWebTLS(grpcServer, certFile, keyFile)
				if err != nil {
					panic(err)
				}
			} else {
				_, err := walletrpc.ServeWebInsecure(grpcServer)
				if err != nil {
					panic(err)
				}
			}
		}

		select {}
	},
}

var rpcServeWebGrpc bool

func init() {
	rootCmd.AddCommand(rpcServerCmd)
	rpcServerCmd.Flags().BoolVar(&rpcServeWebGrpc, "web", false, "Open up a grpc-web interface as well")
	rpcServerCmd.Flags().BoolVarP(&tls, "tls", "t", false, "Encrypt connections with TLS/SSL")
	rpcServerCmd.Flags().StringVarP(&certFile, "tls-cert", "C", "", "TLS certificate file")
	rpcServerCmd.Flags().StringVarP(&keyFile, "tls-key", "K", "", "TLS private key file")
}
