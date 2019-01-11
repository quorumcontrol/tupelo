package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/quorumcontrol/tupelo/wallet/walletrpc"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	gossip3client "github.com/quorumcontrol/tupelo/gossip3/client"
	gossip3remote "github.com/quorumcontrol/tupelo/gossip3/remote"
	gossip3types "github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/spf13/cobra"
)

func unmarshalKeys(keySet interface{}, bytes []byte) error {
	if bytes != nil {
		err := json.Unmarshal(bytes, keySet)
		if err != nil {
			return err
		}
	}

	return nil
}

func loadKeyFile(keySet interface{}, path string, name string) error {
	jsonBytes, err := readConfig(path, name)
	if err != nil {
		return fmt.Errorf("error loading key file: %v", err)
	}

	return unmarshalKeys(keySet, jsonBytes)
}

func loadPublicKeyFile(path string) ([]*PublicKeySet, error) {
	var keySet []*PublicKeySet
	var err error

	err = loadKeyFile(&keySet, path, publicKeyFile)

	return keySet, err
}

func loadPrivateKeyFile(path string) ([]*PrivateKeySet, error) {
	var keySet []*PrivateKeySet
	var err error

	err = loadKeyFile(&keySet, path, privateKeyFile)

	return keySet, err
}

func loadLocalKeys(num int) ([]*PrivateKeySet, []*PublicKeySet, error) {
	privateKeys, err := loadPrivateKeyFile(localConfig)
	if err != nil {
		return nil, nil, fmt.Errorf("error loading private keys: %v", err)
	}

	publicKeys, err := loadPublicKeyFile(localConfig)
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

		err = writeJSONKeys(combinedPrivateKeys, combinedPublicKeys, localConfig)
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
	bootstrapNode.Bootstrap(p2p.BootstrapNodes())
	err = bootstrapNode.WaitForBootstrap(1, 10*time.Second)
	if err != nil {
		panic("error, timed out waiting for bootstrap")
	}
	gossip3remote.NewRouter(bootstrapNode)
	addrs := bootstrapAddresses(bootstrapNode)
	os.Setenv("TUPELO_BOOTSTRAP_NODES", strings.Join(addrs, ","))

	privateKeys, publicKeys, err := loadLocalKeys(nodeCount)
	if err != nil {
		panic(fmt.Sprintf("error generating node keys: %v", err))
	}

	bootstrapPublicKeys = publicKeys
	signers := make([]*gossip3types.Signer, len(privateKeys))
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

func walletPath() string {
	return configDir("wallets")
}

var (
	certFile              string
	keyFile               string
	localNetworkNodeCount int
	remote                bool
	tls                   bool
)

var rpcServerCmd = &cobra.Command{
	Use:   "rpc-server",
	Short: "Launches a Tupelo RPC Server",
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
		if tls {
			panicWithoutTLSOpts()
			walletrpc.ServeTLS(walletStorage, client, certFile, keyFile)
		} else {
			walletrpc.ServeInsecure(walletStorage, client)
		}
	},
}

func init() {
	rootCmd.AddCommand(rpcServerCmd)
	rpcServerCmd.Flags().IntVarP(&localNetworkNodeCount, "local-network", "l", 3, "Run local network with randomly generated keys, specifying number of nodes as argument. Mutually exlusive with bootstrap-*")
	rpcServerCmd.Flags().BoolVarP(&tls, "tls", "t", false, "Encrypt connections with TLS/SSL")
	rpcServerCmd.Flags().StringVarP(&certFile, "tls-cert", "C", "", "TLS certificate file")
	rpcServerCmd.Flags().StringVarP(&keyFile, "tls-key", "K", "", "TLS private key file")
}
