package cmd

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/p2p"
	"github.com/spf13/cobra"
)

var (
	generateNodeKeysCount  int
	generateNodeKeysOutput string
	generateNodeKeysPath   string
)

func generateKeySet(numberOfKeys int) (privateKeys []*PrivateKeySet, publicKeys []*PublicKeySet, err error) {
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

// generateNodeKeysCmd represents the generateNodeKeys command
var generateNodeKeysCmd = &cobra.Command{
	Use:   "generate-node-keys",
	Short: "Generate a new set of node keys",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		privateKeys, publicKeys, err := generateKeySet(generateNodeKeysCount)

		if err != nil {
			panic(err)
		}

		switch generateNodeKeysOutput {
		case "text":
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
		case "json-file":
			publicKeyJson, err := json.Marshal(publicKeys)
			if err != nil {
				panic(fmt.Sprintf("error writing json %v", err))
			}
			err = ioutil.WriteFile(filepath.Join(generateNodeKeysPath, "public-keys.json"), publicKeyJson, 0644)
			if err != nil {
				panic(fmt.Sprintf("error writing file %v", err))
			}

			privateKeyJson, err := json.Marshal(privateKeys)
			if err != nil {
				panic(fmt.Sprintf("error writing json %v", err))
			}
			err = ioutil.WriteFile(filepath.Join(generateNodeKeysPath, "private-keys.json"), privateKeyJson, 0644)
			if err != nil {
				panic(fmt.Sprintf("error writing file %v", err))
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
