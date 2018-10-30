package cmd

import (
	"encoding/json"
	"strings"

	"github.com/quorumcontrol/qc3/signer"
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

func setupLocalNetwork() {
	var err error
	var publicKeys []*PublicKeySet
	var privateKeys []*PrivateKeySet

	privateKeys, publicKeys, err = generateKeySet(localNetworkNodeCount)
	if err != nil {
		panic("Can't generate node keys")
	}
	bootstrapPublicKeys = publicKeys
	signers := make([]*signer.GossipedSigner, len(privateKeys))
	for i, keys := range privateKeys {
		signers[i] = setupGossipNode(keys.EcdsaHexPrivateKey, keys.BlsHexPrivateKey)
	}
}

var rpcServerCmd = &cobra.Command{
	Use:   "rpc-server",
	Short: "Launches a Tupelo RPC Server",
	Run: func(cmd *cobra.Command, args []string) {
		if localNetworkNodeCount > 0 {
			setupLocalNetwork()
		}

		notaryGroup := setupNotaryGroup(storage.NewMemStorage())
		if tls {
			panicWithoutTLSOpts()
			walletrpc.ServeTLS(notaryGroup, certFile, keyFile)
		} else {
			walletrpc.ServeInsecure(notaryGroup)
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
