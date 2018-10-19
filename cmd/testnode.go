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
	"crypto/elliptic"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"os/user"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/signer"
	"github.com/quorumcontrol/storage"
	"github.com/spf13/cobra"
)

var (
	BlsSignKeys []*bls.SignKey
	EcdsaKeys   []*ecdsa.PrivateKey
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

func loadPublicKeyFile(path string) ([]*PublicKeySet, error) {
	var jsonLoadedKeys []*PublicKeySet

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

func bootstrapMembers(keys []*PublicKeySet) (members []*consensus.RemoteNode) {
	for _, keySet := range keys {
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

// testnodeCmd represents the testnode command
var testnodeCmd = &cobra.Command{
	Use:   "test-node [index of key]",
	Short: "Run a testnet node with hardcoded (insecure) keys",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		ecdsaKeyHex := os.Getenv("NODE_ECDSA_KEY_HEX")
		blsKeyHex := os.Getenv("NODE_BLS_KEY_HEX")
		signer := setupGossipNode(ecdsaKeyHex, blsKeyHex)
		stopOnSignal(signer)
	},
}

func setupNotaryGroup(storageAdapter storage.Storage) *consensus.NotaryGroup {
	nodeStore := nodestore.NewStorageBasedStore(storageAdapter)
	group := consensus.NewNotaryGroup("hardcodedprivatekeysareunsafe", nodeStore)
	if group.IsGenesis() {
		testNetMembers := bootstrapMembers(bootstrapPublicKeys)
		log.Debug("Creating gensis state", "nodes", len(testNetMembers))
		group.CreateGenesisState(group.RoundAt(time.Now()), testNetMembers...)
	}

	return group
}

func setupGossipNode(ecdsaKeyHex string, blsKeyHex string) *signer.GossipedSigner {
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(log.LvlDebug), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	ecdsaKey, err := crypto.ToECDSA(hexutil.MustDecode(ecdsaKeyHex))
	// TODO: rather than overwrite curve, reconstruct ECDSA key manually
	ecdsaKey.PublicKey.Curve = elliptic.P256()
	if err != nil {
		panic("error fetching ecdsa key - set env variable NODE_ECDSA_KEY_HEX")
	}

	blsKey := bls.BytesToSignKey(hexutil.MustDecode(blsKeyHex))

	id := consensus.EcdsaToPublicKey(&ecdsaKey.PublicKey).Id
	log.Info("starting up a test GOSSIP node", "id", id)

	os.MkdirAll(".storage", 0700)
	badgerStorage := storage.NewBadgerStorage(filepath.Join(".storage", "testnode-chains-"+id))
	node := network.NewNode(ecdsaKey)

	group := setupNotaryGroup(badgerStorage)

	gossipedSigner := signer.NewGossipedSigner(node, group, badgerStorage, blsKey)
	gossipedSigner.Start()

	return gossipedSigner
}

func stopOnSignal(signers ...*signer.GossipedSigner) {
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		fmt.Println(sig)
		for _, signer := range signers {
			signer.Stop()
		}
		done <- true
	}()
	fmt.Println("awaiting signal")
	<-done
	fmt.Println("exiting")
}

func init() {
	rootCmd.AddCommand(testnodeCmd)
	testnodeCmd.Flags().StringVarP(&bootstrapPublicKeysFile, "bootstrap-keys", "k", "", "which keys to bootstrap the notary groups with")
}
