package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/spf13/cobra"
)

const localConfig = "local-network"

func loadKeyFile(keySet interface{}, namespace string, name string) error {
	jsonBytes, err := readConfig(namespace, name)
	if err != nil {
		return fmt.Errorf("error loading key file: %v", err)
	}

	if jsonBytes != nil {
		err = json.Unmarshal(jsonBytes, keySet)
		if err != nil {
			return err
		}
	}

	return nil
}

func loadPublicKeyFile(namespace string) ([]*PublicKeySet, error) {
	var keySet []*PublicKeySet
	var err error

	err = loadKeyFile(&keySet, namespace, publicKeyFile)

	return keySet, err
}

func loadPrivateKeyFile(namespace string) ([]*PrivateKeySet, error) {
	var keySet []*PrivateKeySet
	var err error

	err = loadKeyFile(&keySet, namespace, privateKeyFile)

	return keySet, err
}

func loadKeys(network string, num int) ([]*PrivateKeySet, []*PublicKeySet, error) {
	privateKeys, err := loadPrivateKeyFile(network)
	if err != nil {
		return nil, nil, fmt.Errorf("error loading private keys: %v", err)
	}

	publicKeys, err := loadPublicKeyFile(network)
	if err != nil {
		return nil, nil, fmt.Errorf("error loading public keys: %v", err)
	}

	savedKeyCount := len(privateKeys)
	if savedKeyCount < num {
		extraPrivateKeys, extraPublicKeys, err := generateKeySets(num - savedKeyCount)
		if err != nil {
			return nil, nil, fmt.Errorf("error generating extra node keys: %v", err)
		}

		combinedPrivateKeys := append(privateKeys, extraPrivateKeys...)
		combinedPublicKeys := append(publicKeys, extraPublicKeys...)

		err = writeJSONKeys(combinedPrivateKeys, combinedPublicKeys, network)
		if err != nil {
			return nil, nil, fmt.Errorf("error writing extra node keys: %v", err)
		}

		return combinedPrivateKeys, combinedPublicKeys, nil
	} else if savedKeyCount > num {
		return privateKeys[:num], publicKeys[:num], nil
	} else {
		return privateKeys, publicKeys, nil
	}
}

func setupLocalNetwork(ctx context.Context, nodeCount int) {
	key, _ := crypto.GenerateKey()
	bootstrapNode, err := p2p.NewLibP2PHost(ctx, key, 0)
	if err != nil {
		panic(fmt.Sprintf("error generating bootstrap node: %v", err))
	}
	addrs := bootstrapAddresses(bootstrapNode)
	os.Setenv("TUPELO_BOOTSTRAP_NODES", strings.Join(addrs, ","))

	privateKeys, publicKeys, err := loadKeys(localConfig, nodeCount)
	if err != nil {
		panic(fmt.Sprintf("error generating node keys: %v", err))
	}

	bootstrapPublicKeys = publicKeys
	signers := make([]*types.Signer, len(privateKeys))
	for i, keys := range privateKeys {
		log.Info("setting up gossip node")
		signers[i] = setupGossipNode(ctx, keys.EcdsaHexPrivateKey, keys.BlsHexPrivateKey, localConfig, 0)
	}
	go func() {
		<-ctx.Done()
		for _, signer := range signers {
			signer.Actor.Poison()
		}
	}()

}

func bootstrapAddresses(bootstrapHost p2p.Node) []string {
	addresses := bootstrapHost.Addresses()
	for _, addr := range addresses {
		addrStr := addr.String()
		if strings.Contains(addrStr, "127.0.0.1") {
			return []string{addrStr}
		}
	}
	return nil
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

var (
	tls                   bool
	certFile              string
	keyFile               string
	localNetworkNodeCount int
)

var rpcServerCmd = &cobra.Command{
	Use:   "rpc-server",
	Short: "Launches a Tupelo RPC Server",
	Run: func(cmd *cobra.Command, args []string) {
		if localNetworkNodeCount > 0 {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			setupLocalNetwork(ctx, localNetworkNodeCount)
		}
		<-make(chan struct{})
		// walletStorage := configDir("wallets").Path
		// notaryGroup := setupNotaryGroup(storage.NewMemStorage())
		// client := gossip2client.NewGossipClient(notaryGroup, bootstrapAddrs)
		// if tls {
		// 	panicWithoutTLSOpts()
		// 	walletrpc.ServeTLS(walletStorage, notaryGroup, client, certFile, keyFile)
		// } else {
		// 	walletrpc.ServeInsecure(walletStorage, notaryGroup, client)
		// }
	},
}

func init() {
	rootCmd.AddCommand(rpcServerCmd)
	rpcServerCmd.Flags().StringVarP(&bootstrapPublicKeysFile, "bootstrap-keys", "k", "", "which public keys to bootstrap the notary groups with")
	rpcServerCmd.Flags().IntVarP(&localNetworkNodeCount, "local-network", "l", 3, "Run local network with randomly generated keys, specifying number of nodes as argument. Mutually exlusive with bootstrap-*")
	rpcServerCmd.Flags().BoolVarP(&tls, "tls", "t", false, "Encrypt connections with TLS/SSL")
	rpcServerCmd.Flags().StringVarP(&certFile, "tls-cert", "C", "", "TLS certificate file")
	rpcServerCmd.Flags().StringVarP(&keyFile, "tls-key", "K", "", "TLS private key file")
}
