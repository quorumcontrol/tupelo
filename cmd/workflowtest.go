package cmd

// import (
// 	"fmt"
// 	"io/ioutil"
// 	"os"
// 	"time"

// 	"github.com/quorumcontrol/chaintree/chaintree"
// 	"github.com/quorumcontrol/chaintree/nodestore"
// 	"github.com/quorumcontrol/storage"
// 	"github.com/quorumcontrol/tupelo/consensus"
// 	"github.com/quorumcontrol/tupelo/gossip2client"
// 	"github.com/quorumcontrol/tupelo/p2p"
// 	"github.com/quorumcontrol/tupelo/wallet"
// 	"github.com/spf13/cobra"
// )

// func smokeTestNetwork() (bool, string) {
// 	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
// 	group := consensus.NewNotaryGroup("hardcodedprivatekeysareunsafe", nodeStore)
// 	if group.IsGenesis() {
// 		testNetMembers := bootstrapMembers(bootstrapPublicKeys)
// 		group.CreateGenesisState(group.RoundAt(time.Now()), testNetMembers...)
// 	}

// 	file, err := ioutil.TempFile("/tmp", "workflowtestwallet")
// 	if err != nil {
// 		return false, "Couldn't create file wallet"
// 	}
// 	defer os.Remove(file.Name())

// 	wallet := wallet.NewFileWallet(file.Name())
// 	wallet.CreateIfNotExists("thisisaninsecuretestnetwallet")
// 	client := gossip2client.NewGossipClient(group, p2p.BootstrapNodes())

// 	key, err := wallet.GenerateKey()
// 	if err != nil {
// 		return false, fmt.Sprintf("error generating key: %v", err)
// 	}

// 	chain, err := consensus.NewSignedChainTree(key.PublicKey, wallet.NodeStore())
// 	if err != nil {
// 		return false, fmt.Sprintf("error generating chain: %v", err)
// 	}
// 	wallet.SaveChain(chain)

// 	var remoteTip string
// 	if !chain.IsGenesis() {
// 		remoteTip = chain.Tip().String()
// 	}

// 	resp, err := client.PlayTransactions(chain, key, remoteTip, []*chaintree.Transaction{
// 		{
// 			Type: consensus.TransactionTypeSetData,
// 			Payload: consensus.SetDataPayload{
// 				Path:  "this/is/a/test/path",
// 				Value: "somevalue",
// 			},
// 		},
// 	})

// 	if err != nil {
// 		return false, fmt.Sprintf("error playing transaction: %v", err)
// 	} else if resp.Tip.String() == "" {
// 		return false, "Tip was not produced"
// 	} else {
// 		return true, ""
// 	}
// }

// // workflowtest represents the shell command
// var workflowtest = &cobra.Command{
// 	Use:    "workflowtest",
// 	Short:  "runs a set of operations against a network to confirm its working",
// 	Hidden: true,
// 	Run: func(cmd *cobra.Command, args []string) {
// 		success, err := smokeTestNetwork()

// 		if success {
// 			os.Exit(0)
// 		} else {
// 			cmd.Print(err)
// 			os.Exit(1)
// 		}
// 	},
// }

// func init() {
// 	rootCmd.AddCommand(workflowtest)
// 	workflowtest.Flags().StringVarP(&bootstrapPublicKeysFile, "bootstrap-keys", "k", "", "which keys to bootstrap the notary groups with")
// }
