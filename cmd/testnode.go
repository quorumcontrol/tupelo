// Copyright © 2018 NAME HERE <EMAIL ADDRESS>
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package cmd

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/signer"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/spf13/cobra"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
)

// These are private keys and they are PURPOSEFULLY checked in
// no testnode should be used to secure anything.
var blsHexKeys = []string{
	"0x062826fc0f8034ce63f160dfa30fca1c11cb66cdbdd0524b0a5103539c44e153",
	"0x047490c1cacdc0e1da3694ce4aa7610e6c094720fe245d860bc6b950fe01af64",
	"0x23d13849867754cff263189bb4b039a7f5fc6c7a63a3b95b31a807fd1a9d2a37",
}

// These are private keys and they are PURPOSEFULLY checked in
// no testnode should be used to secure anything.
var ecdsaHexKeys = []string{
	"0xa457195d256894b7a44400b2ec844a5c58ecadaea5da271abdf0abad1f30c1f0",
	"0x6ed4da1806f0c8587644e6a27ad11b2053d7b7e03d41ffec589a217fd839eae6",
	"0xef8d350dffaed798e846c0b84316023e28b5128cb15158aafdafc5738b6a745b",
}

var BlsSignKeys []*bls.SignKey
var EcdsaKeys []*ecdsa.PrivateKey
var TestNetPublicKeys []consensus.PublicKey
var TestNetGroup *consensus.Group

func init() {
	BlsSignKeys = make([]*bls.SignKey, len(blsHexKeys))
	EcdsaKeys = make([]*ecdsa.PrivateKey, len(ecdsaHexKeys))

	for i, hex := range blsHexKeys {
		BlsSignKeys[i] = bls.BytesToSignKey(hexutil.MustDecode(hex))
	}

	for i, hex := range ecdsaHexKeys {
		key, err := crypto.ToECDSA(hexutil.MustDecode(hex))
		if err != nil {
			panic("error converting to key")
		}
		EcdsaKeys[i] = key
	}

	TestNetPublicKeys = make([]consensus.PublicKey, len(BlsSignKeys))
	for i, key := range BlsSignKeys {
		TestNetPublicKeys[i] = consensus.BlsKeyToPublicKey(key.MustVerKey())
	}

	TestNetGroup = consensus.GroupFromPublicKeys(TestNetPublicKeys)
}

var nodeIndex int

// testnodeCmd represents the testnode command
var testnodeCmd = &cobra.Command{
	Use:   "test-node [index of key]",
	Short: "Run a testnet node with hardcoded (insecure) keys",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("starting up a test node with index: %v\n", nodeIndex)

		log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(log.LvlDebug), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

		os.MkdirAll(".storage", 0700)

		boltStorage := storage.NewBoltStorage(filepath.Join(".storage", "testnode-chains-"+strconv.Itoa(nodeIndex)))

		sign := &signer.Signer{
			Storage: boltStorage,
			Group:   TestNetGroup,
			Id:      consensus.BlsVerKeyToAddress(BlsSignKeys[nodeIndex].MustVerKey().Bytes()).String(),
			SignKey: BlsSignKeys[nodeIndex],
			VerKey:  BlsSignKeys[nodeIndex].MustVerKey(),
		}

		node := network.NewNode(EcdsaKeys[nodeIndex])

		networkedSigner := signer.NewNetworkedSigner(node, sign)
		networkedSigner.Start()

		sigs := make(chan os.Signal, 1)
		done := make(chan bool, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			sig := <-sigs
			fmt.Println()
			fmt.Println(sig)
			networkedSigner.Stop()
			done <- true
		}()
		fmt.Println("awaiting signal")
		<-done
		fmt.Println("exiting")
	},
}

func init() {
	rootCmd.AddCommand(testnodeCmd)
	testnodeCmd.Flags().IntVarP(&nodeIndex, "index", "i", 0, "which key to use")
}
