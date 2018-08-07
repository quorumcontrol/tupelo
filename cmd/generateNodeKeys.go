package cmd

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/spf13/cobra"
)

// generateNodeKeysCmd represents the generateNodeKeys command
var generateNodeKeysCmd = &cobra.Command{
	Use:   "generate-node-keys",
	Short: "Generate a new set of node keys",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		blsKey, err := bls.NewSignKey()
		if err != nil {
			panic("error generating bls")
		}
		ecdsa, err := crypto.GenerateKey()
		if err != nil {
			panic("error generating ecdsa")
		}

		fmt.Printf("bls: '%v'\necdsa: '%v'\n", hexutil.Encode(blsKey.Bytes()), hexutil.Encode(crypto.FromECDSA(ecdsa)))
	},
}

func init() {
	rootCmd.AddCommand(generateNodeKeysCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// generateNodeKeysCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// generateNodeKeysCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}
