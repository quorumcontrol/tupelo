package cmd

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/quorumcontrol/qc3/gossip2"
	"github.com/quorumcontrol/qc3/gossip2client"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/p2p"
	"github.com/quorumcontrol/qc3/wallet/walletrpc"
	"github.com/quorumcontrol/storage"
	"github.com/spf13/cobra"
)

var (
	tls                   bool
	certFile              string
	keyFile               string
	localNetworkNodeCount int
)

func loadPrivateKeyFile(path string) ([]*PrivateKeySet, error) {
	var jsonLoadedKeys []*PrivateKeySet

	jsonBytes, err := loadJSON(path)
	if err != nil {
		return nil, err
	}

	err = json.Unmarshal(jsonBytes, &jsonLoadedKeys)
	if err != nil {
		return nil, err
	}

	return jsonLoadedKeys, nil
}

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

func setupLocalNetwork(ctx context.Context) (bootstrapAddrs []string) {
	var err error
	var publicKeys []*PublicKeySet
	var privateKeys []*PrivateKeySet

	privateKeys, publicKeys, err = generateKeySet(localNetworkNodeCount)
	if err != nil {
		panic("Can't generate node keys")
	}
	bootstrapPublicKeys = publicKeys
	signers := make([]*gossip2.GossipNode, len(privateKeys))
	for i, keys := range privateKeys {
		signers[i] = setupGossipNode(ctx, keys.EcdsaHexPrivateKey, keys.BlsHexPrivateKey, 0)
	}
	// Use first signer as bootstrap node
	bootstrapAddrs = bootstrapAddresses(signers[0].Host)
	// Have rest of signers bootstrap to node 0
	for i := 1; i < len(signers); i++ {
		signers[i].Host.Bootstrap(bootstrapAddrs)
	}
	// Give a chance for everything to bootstrap, then start
	time.Sleep(100 * time.Millisecond)
	for i := 0; i < len(signers); i++ {
		go signers[i].Start()
	}

	return bootstrapAddrs
}

func bootstrapAddresses(bootstrapHost *p2p.Host) []string {
	addresses := bootstrapHost.Addresses()
	for _, addr := range addresses {
		addrStr := addr.String()
		if strings.Contains(addrStr, "127.0.0.1") {
			return []string{addrStr}
		}
	}
	return nil
}

var rpcServerCmd = &cobra.Command{
	Use:   "rpc-server",
	Short: "Launches a Tupelo RPC Server",
	Run: func(cmd *cobra.Command, args []string) {
		bootstrapAddrs := network.BootstrapNodes()
		if localNetworkNodeCount > 0 {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			bootstrapAddrs = setupLocalNetwork(ctx)
		}

		notaryGroup := setupNotaryGroup(storage.NewMemStorage())
		client := gossip2client.NewGossipClient(notaryGroup, bootstrapAddrs)
		if tls {
			panicWithoutTLSOpts()
			walletrpc.ServeTLS(notaryGroup, client, certFile, keyFile)
		} else {
			walletrpc.ServeInsecure(notaryGroup, client)
		}
	},
}

func init() {
	rootCmd.AddCommand(rpcServerCmd)
	rpcServerCmd.Flags().StringVarP(&bootstrapPublicKeysFile, "bootstrap-keys", "k", "", "which public keys to bootstrap the notary groups with")
	rpcServerCmd.Flags().IntVarP(&localNetworkNodeCount, "local-network", "l", 0, "Run local network with randomly generated keys, specifying number of nodes as argument. Mutually exlusive with bootstrap-*")
	rpcServerCmd.Flags().BoolVarP(&tls, "tls", "t", false, "Encrypt connections with TLS/SSL")
	rpcServerCmd.Flags().StringVarP(&certFile, "tls-cert", "C", "", "TLS certificate file")
	rpcServerCmd.Flags().StringVarP(&keyFile, "tls-key", "K", "", "TLS private key file")
}
