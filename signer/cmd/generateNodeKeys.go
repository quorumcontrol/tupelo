package cmd

import (
	"encoding/json"
	"fmt"

	"github.com/quorumcontrol/tupelo/signer/nodebuilder"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo/sdk/bls"
	"github.com/quorumcontrol/tupelo/sdk/p2p"
	"github.com/spf13/cobra"
)

var (
	generateNodeKeysCount  int
	generateNodeKeysOutput string
	generateNodeKeysPath   string
)

func generateKeySets(numberOfKeys int) (privateKeys []*nodebuilder.LegacyPrivateKeySet, publicKeys []*nodebuilder.LegacyPublicKeySet, err error) {
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

		privateKeys = append(privateKeys, &nodebuilder.LegacyPrivateKeySet{
			BlsHexPrivateKey:   hexutil.Encode(blsKey.Bytes()),
			EcdsaHexPrivateKey: hexutil.Encode(crypto.FromECDSA(ecdsaKey)),
		})

		publicKeys = append(publicKeys, &nodebuilder.LegacyPublicKeySet{
			BlsHexPublicKey:   hexutil.Encode(blsKey.MustVerKey().Bytes()),
			EcdsaHexPublicKey: hexutil.Encode(crypto.FromECDSAPub(&ecdsaKey.PublicKey)),
			PeerIDBase58Key:   peerID.Pretty(),
		})
	}

	return privateKeys, publicKeys, err
}

func printTextKeys(privateKeys []*nodebuilder.LegacyPrivateKeySet, publicKeys []*nodebuilder.LegacyPublicKeySet) {
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

type printJSONKey struct {
	*nodebuilder.LegacyPrivateKeySet
	*nodebuilder.LegacyPublicKeySet
}

func printJSONKeys(privateKeys []*nodebuilder.LegacyPrivateKeySet, publicKeys []*nodebuilder.LegacyPublicKeySet) {
	toPrint := make([]*printJSONKey, len(privateKeys))
	for i := range privateKeys {
		toPrint[i] = &printJSONKey{
			privateKeys[i],
			publicKeys[i],
		}
	}

	keysJSON, err := json.Marshal(toPrint)
	if err != nil {
		panic(fmt.Errorf("error Error marshaling keys: %v", err))
	}

	fmt.Println(string(keysJSON))
}

func writeJSONKeys(privateKeys []*nodebuilder.LegacyPrivateKeySet, publicKeys []*nodebuilder.LegacyPublicKeySet, path string) error {
	publicKeyJson, err := json.Marshal(publicKeys)
	if err != nil {
		return fmt.Errorf("Error marshaling public keys: %v", err)
	}

	err = writeFile(path, publicKeyFile, publicKeyJson)
	if err != nil {
		return fmt.Errorf("error writing public keys: %v", err)
	}

	privateKeyJson, err := json.Marshal(privateKeys)
	if err != nil {
		return fmt.Errorf("Error marshaling private keys: %v", err)
	}

	err = writeFile(path, privateKeyFile, privateKeyJson)
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
		case "json":
			printJSONKeys(privateKeys, publicKeys)
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
}
