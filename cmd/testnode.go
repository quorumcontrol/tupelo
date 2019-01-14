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
	"context"
	"crypto/ecdsa"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	logging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/bls"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/quorumcontrol/tupelo/gossip2"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/spf13/cobra"
)

var (
	BlsSignKeys  []*bls.SignKey
	EcdsaKeys    []*ecdsa.PrivateKey
	testnodePort int
)

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
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		logging.SetLogLevel("gossip", "ERROR")
		ecdsaKeyHex := os.Getenv("TUPELO_NODE_ECDSA_KEY_HEX")
		blsKeyHex := os.Getenv("TUPELO_NODE_BLS_KEY_HEX")
		signer := setupGossipNode(ctx, ecdsaKeyHex, blsKeyHex, "distributed-network", testnodePort)
		signer.Host.Bootstrap(p2p.BootstrapNodes())
		go signer.Start()
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

func setupGossipNode(ctx context.Context, ecdsaKeyHex string, blsKeyHex string, namespace string, port int) *gossip2.GossipNode {
	ecdsaKey, err := crypto.ToECDSA(hexutil.MustDecode(ecdsaKeyHex))
	if err != nil {
		panic("error fetching ecdsa key - set env variable TUPELO_NODE_ECDSA_KEY_HEX")
	}

	blsKey := bls.BytesToSignKey(hexutil.MustDecode(blsKeyHex))

	id := consensus.EcdsaToPublicKey(&ecdsaKey.PublicKey).Id
	log.Info("starting up a test node", "id", id)

	storagePath := configDir(namespace)
	os.MkdirAll(storagePath, 0700)

	db := filepath.Join(storagePath, id+"-chains")
	badgerStorage, err := storage.NewBadgerStorage(db)
	if err != nil {
		panic(fmt.Sprintf("error creating storage: %v", err))
	}

	p2pHost, err := p2p.NewLibP2PHost(ctx, ecdsaKey, port)
	if err != nil {
		panic("error setting up p2p host")
	}
	group := setupNotaryGroup(storage.NewMemStorage())

	gossipedSigner := gossip2.NewGossipNode(ecdsaKey, blsKey, p2pHost, badgerStorage)
	gossipedSigner.Group = group

	return gossipedSigner
}

func stopOnSignal(signers ...*gossip2.GossipNode) {
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
	testnodeCmd.Flags().IntVarP(&testnodePort, "port", "p", 0, "what port will the node listen on")
}
