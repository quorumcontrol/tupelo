package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/log"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/gossip2"
	"github.com/quorumcontrol/tupelo/gossip2client"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/quorumcontrol/tupelo/wallet/walletrpc"
	"github.com/spf13/cobra"
)


func expandHomePath(path string) (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("error getting current user: %v", err)
	}
	homeDir := currentUser.HomeDir

	if path[:2] == "~/" {
		path = filepath.Join(homeDir, path[2:])
	}
	return path, nil
}

func loadJSON(path string) ([]byte, error) {
	if path == "" {
		return nil, nil
	}
	modPath, err := expandHomePath(path)
	if err != nil {
		return nil, err
	}
	_, err = os.Stat(modPath)
	if err != nil {
		return nil, err
	}

	return ioutil.ReadFile(modPath)
}

func loadKeyFile(keySet interface{}, path string) error {
	jsonBytes, err := loadJSON(path)
	if err != nil {
		return err
	}

	err = json.Unmarshal(jsonBytes, keySet)
	if err != nil {
		return err
	}

	return nil
}

func loadPublicKeyFile(path string) ([]*PublicKeySet, error) {
	var keySet []*PublicKeySet
	err := loadKeyFile(&keySet, publicKeyFile(path))

	return keySet, err
}

func loadPrivateKeyFile(path string) ([]*PrivateKeySet, error) {
	var keySet []*PrivateKeySet
	err := loadKeyFile(&keySet, privateKeyFile(path))

	return keySet, err
}

func loadKeys(path string, num int) ([]*PrivateKeySet, []*PublicKeySet, error) {
	privateKeys, err := loadPrivateKeyFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("Error loading private keys: %v", err)
	}

	publicKeys, err := loadPublicKeyFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("Error loading public keys: %v", err)
	}

	savedKeyCount := len(privateKeys)
	if savedKeyCount < num {
		extraPrivateKeys, extraPublicKeys, err := generateKeySets(num - savedKeyCount)
		if err != nil {
			return nil, nil, fmt.Errorf("Error generating extra node keys: %v", err)
		}

		combinedPrivateKeys := append(privateKeys, extraPrivateKeys...)
		combinedPublicKeys := append(publicKeys, extraPublicKeys...)

		err = writeJSONKeys(combinedPrivateKeys, combinedPublicKeys, path)
		if err != nil {
			return nil, nil, fmt.Errorf("Error writing extra node keys: %v", err)
		}

		return combinedPrivateKeys, combinedPublicKeys, nil
	} else if savedKeyCount > num {
		return privateKeys[:num], publicKeys[:num], nil
	} else {
		return privateKeys, publicKeys, nil
	}
}

func setupLocalNetwork(ctx context.Context, configPath string, nodeCount int) (bootstrapAddrs []string) {
	var err error
	var publicKeys []*PublicKeySet
	var privateKeys []*PrivateKeySet

	privateKeys, publicKeys, err = loadKeys(configPath, nodeCount)
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

var rpcServerCmd = &cobra.Command{
	Use:   "rpc-server",
	Short: "Launches a Tupelo RPC Server",
	Run: func(cmd *cobra.Command, args []string) {

		bootstrapAddrs := p2p.BootstrapNodes()
		if localNetworkNodeCount > 0 {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			bootstrapAddrs = setupLocalNetwork(ctx, localConfigPath, localNetworkNodeCount)
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

var (
	tls                   bool
	certFile              string
	keyFile               string
	localNetworkNodeCount int
	localConfigPath       string
)

func defaultCfgPath() string {
	usr, err := user.Current()
	if err != nil {
		log.Warn("Error finding user: %v", err)
	}
	return filepath.Join(usr.HomeDir, ".tupelo/local_network")
}

func init() {
	rootCmd.AddCommand(rpcServerCmd)
	rpcServerCmd.Flags().StringVarP(&bootstrapPublicKeysFile, "bootstrap-keys", "k", "", "which public keys to bootstrap the notary groups with")
	rpcServerCmd.Flags().IntVarP(&localNetworkNodeCount, "local-network", "l", 3, "Run local network with randomly generated keys, specifying number of nodes as argument. Mutually exlusive with bootstrap-*")
	rpcServerCmd.Flags().BoolVarP(&tls, "tls", "t", false, "Encrypt connections with TLS/SSL")
	rpcServerCmd.Flags().StringVarP(&localConfigPath, "config-path", "c", defaultCfgPath(), "Local network configuration")
	rpcServerCmd.Flags().StringVarP(&certFile, "tls-cert", "C", "", "TLS certificate file")
	rpcServerCmd.Flags().StringVarP(&keyFile, "tls-key", "K", "", "TLS private key file")
}
