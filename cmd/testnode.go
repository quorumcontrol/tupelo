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
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/quorumcontrol/chaintree/nodestore"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/signer"
	"github.com/quorumcontrol/storage"
	"github.com/spf13/cobra"
)

var BlsSignKeys []*bls.SignKey
var EcdsaKeys []*ecdsa.PrivateKey

type KeySet struct {
	BlsHexPublicKey   string `json:"blsHexPublicKey,omitempty"`
	EcdsaHexPublicKey string `json:"ecdsaHexPublicKey,omitempty"`
}

func bootstrapMembers(path string) (members []*consensus.RemoteNode) {
	var jsonLoadedKeys []*KeySet

	if _, err := os.Stat(path); err == nil {
		jsonBytes, err := ioutil.ReadFile(path)

		if err != nil {
			fmt.Printf("Error reading from %v: %v", path, err)
			return nil
		}
		json.Unmarshal(jsonBytes, &jsonLoadedKeys)
	}

	for _, keySet := range jsonLoadedKeys {
		blsPubKey := consensus.PublicKey{
			PublicKey: hexutil.MustDecode(keySet.BlsHexPublicKey),
			Type:      consensus.KeyTypeBLSGroupSig,
		}
		blsPubKey.Id = consensus.PublicKeyToAddr(&blsPubKey)

		ecdsaPubKey := consensus.PublicKey{
			PublicKey: hexutil.MustDecode(keySet.EcdsaHexPublicKey),
			Type:      consensus.KeyTypeSecp256k1,
		}
		ecdsaPubKey.Id = consensus.PublicKeyToAddr(&ecdsaPubKey)

		members = append(members, consensus.NewRemoteNode(blsPubKey, ecdsaPubKey))
	}

	return members
}

var bootstrapKeysFile string

// testnodeCmd represents the testnode command
var testnodeCmd = &cobra.Command{
	Use:   "test-node [index of key]",
	Short: "Run a testnet node with hardcoded (insecure) keys",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		setupGossipNode()
	},
}

func setupGossipNode() {
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(log.LvlDebug), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	ecdsaKey, err := crypto.ToECDSA(hexutil.MustDecode(os.Getenv("NODE_ECDSA_KEY_HEX")))
	if err != nil {
		panic("error fetching ecdsa key - set env variable NODE_ECDSA_KEY_HEX")
	}

	blsKey := bls.BytesToSignKey(hexutil.MustDecode(os.Getenv("NODE_BLS_KEY_HEX")))

	id := consensus.EcdsaToPublicKey(&ecdsaKey.PublicKey).Id
	log.Info("starting up a test GOSSIP node", "id", id)

	os.MkdirAll(".storage", 0700)
	boltStorage := storage.NewBoltStorage(filepath.Join(".storage", "testnode-chains-"+id))
	node := network.NewNode(ecdsaKey)

	group := consensus.NewNotaryGroup("hardcodedprivatekeysareunsafe", nodestore.NewStorageBasedStore(boltStorage))
	if group.IsGenesis() {
		testNetMembers := bootstrapMembers(bootstrapKeysFile)
		log.Debug("Creating gensis state", "nodes", len(testNetMembers))
		group.CreateGenesisState(group.RoundAt(time.Now()), testNetMembers...)
	}
	gossipedSigner := signer.NewGossipedSigner(node, group, boltStorage, blsKey)
	gossipedSigner.Start()

	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		fmt.Println(sig)
		gossipedSigner.Stop()
		done <- true
	}()
	fmt.Println("awaiting signal")
	<-done
	fmt.Println("exiting")
}

func init() {
	rootCmd.AddCommand(testnodeCmd)
	testnodeCmd.Flags().StringVarP(&bootstrapKeysFile, "bootstrap-keys", "k", "", "which keys to bootstrap the notary groups with")
}
