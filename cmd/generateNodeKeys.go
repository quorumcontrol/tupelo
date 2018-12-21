package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo/bls"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/spf13/cobra"
)

var (
	generateNodeKeysCount  int
	generateNodeKeysOutput string
	generateNodeKeysPath   string
)

func generateKeySets(numberOfKeys int) (privateKeys []*PrivateKeySet, publicKeys []*PublicKeySet, err error) {
	for i := 1; i <= numberOfKeys; i++ {
		blsKey, err := bls.NewSignKey()
		if err != nil {
			return nil, nil, err
		}
		ecdsaKey, err := crypto.GenerateKey()
		if err != nil {
			return nil, nil, err
		}
		peerID, err := p2p.PeerIDFromPublicKey(&ecdsaKey.PublicKey)
		if err != nil {
			return nil, nil, err
		}

		privateKeys = append(privateKeys, &PrivateKeySet{
			BlsHexPrivateKey:   hexutil.Encode(blsKey.Bytes()),
			EcdsaHexPrivateKey: hexutil.Encode(crypto.FromECDSA(ecdsaKey)),
		})

		publicKeys = append(publicKeys, &PublicKeySet{
			BlsHexPublicKey:   hexutil.Encode(consensus.BlsKeyToPublicKey(blsKey.MustVerKey()).PublicKey),
			EcdsaHexPublicKey: hexutil.Encode(consensus.EcdsaToPublicKey(&ecdsaKey.PublicKey).PublicKey),
			PeerIDBase58Key:   peerID.Pretty(),
		})
	}

	return privateKeys, publicKeys, err
}

func printTextKeys(privateKeys []*PrivateKeySet, publicKeys []*PublicKeySet) {
	for i := 1; i <= len(privateKeys); i++ {
		fmt.Printf("================ Key %v ================\n", i)
		fmt.Printf(
			"bls: '%v'\nbls public: '%v'\necdsa: '%v'\necdsa public: '%v'\npeer id: '%v'",
			privateKeys[0].BlsHexPrivateKey,
			publicKeys[0].BlsHexPublicKey,
			privateKeys[0].EcdsaHexPrivateKey,
			publicKeys[0].EcdsaHexPublicKey,
			publicKeys[0].PeerIDBase58Key,
		)
	}
}

const (
	publicKeyFile  = "public-keys.json"
	privateKeyFile = "private-keys.json"
)

func writeJSONKeys(privateKeys []*PrivateKeySet, publicKeys []*PublicKeySet, namespace string) error {
	publicKeyJson, err := json.Marshal(publicKeys)
	if err != nil {
		return fmt.Errorf("Error marshaling public keys: %v", err)
	}

	err = writeConfig(namespace, publicKeyFile, publicKeyJson)
	if err != nil {
		return fmt.Errorf("error writing public keys: %v", err)
	}

	privateKeyJson, err := json.Marshal(privateKeys)
	if err != nil {
		return fmt.Errorf("Error marshaling private keys: %v", err)
	}

	err = writeConfig(namespace, privateKeyFile, privateKeyJson)
	if err != nil {
		return fmt.Errorf("error writing private keys: %v", err)
	}

	return nil
}

// generateNodeKeysCmd represents the generateNodeKeys command
var generateNodeKeysCmd = &cobra.Command{
	Use:   "generate-node-keys",
	Short: "Generate a new set of node keys",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		privateKeys, publicKeys, err := generateKeySets(generateNodeKeysCount)
		if err != nil {
			panic(fmt.Sprintf("error generating key sets: %v", err))
		}

		switch generateNodeKeysOutput {
		case "text":
			printTextKeys(privateKeys, publicKeys)
		case "json-file":
			err := writeJSONKeys(privateKeys, publicKeys, generateNodeKeysPath)
			if err != nil {
				panic(fmt.Sprintf("error writing json file: %v", err))
			}
		default:
			panic(fmt.Sprintf("output=%v type is not supported", generateNodeKeysOutput))
		}
	},
}

func init() {
	rootCmd.AddCommand(generateNodeKeysCmd)
	generateNodeKeysCmd.Flags().IntVarP(&generateNodeKeysCount, "count", "c", 1, "how many keys to generate")
	generateNodeKeysCmd.Flags().StringVarP(&generateNodeKeysOutput, "output", "o", "text", "format for keys output (default text): text, json-file")
	generateNodeKeysCmd.Flags().StringVarP(&generateNodeKeysPath, "path", "p", ".", "directory to store files if using json")

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// generateNodeKeysCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// generateNodeKeysCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
