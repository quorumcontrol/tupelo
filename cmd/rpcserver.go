package cmd

import (
	"encoding/json"

	"github.com/quorumcontrol/qc3/signer"
	"github.com/quorumcontrol/qc3/wallet/walletrpc"
	"github.com/spf13/cobra"
)

var bootstrapPrivateKeysFile string
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

var rpcServerCmd = &cobra.Command{
	Use:   "rpc-server",
	Short: "Launches a Tupelo RPC Server",
	Run: func(cmd *cobra.Command, args []string) {
		// memStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
		// notaryGroup := consensus.NewNotaryGroup("hardcodedprivatekeysareunsafe", memStore)
		// if notaryGroup.IsGenesis() {
		//	testNetMembers := bootstrapMembers(bootstrapPublicKeysFile)
		//	fmt.Printf("Bootstrapping notary group with %v nodes\n", len(testNetMembers))
		//	notaryGroup.CreateGenesisState(notaryGroup.RoundAt(time.Now()), testNetMembers...)
		// }
		notaryGroup := setupNotaryGroup()
		privateKeys, _ := loadPrivateKeyFile(bootstrapPrivateKeysFile)
		signers := make([]*signer.GossipedSigner, len(privateKeys))
		for i, keys := range privateKeys {
			signers[i] = setupGossipNode(keys.EcdsaHexPrivateKey, keys.BlsHexPrivateKey, notaryGroup)
		}
		walletrpc.Serve(notaryGroup)
	},
}

func init() {
	rootCmd.AddCommand(rpcServerCmd)
	rpcServerCmd.Flags().StringVarP(&bootstrapPublicKeysFile, "bootstrap-keys", "k", "", "which public keys to bootstrap the notary groups with")
	rpcServerCmd.Flags().StringVarP(&bootstrapPrivateKeysFile, "bootstrap-private-keys", "s", "", "which private keys to bootstrap the notary groups with")
}
