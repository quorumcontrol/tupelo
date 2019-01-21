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
	"bytes"
	"context"
	"crypto/ecdsa"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	logging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/bls"
	gossip3actors "github.com/quorumcontrol/tupelo/gossip3/actors"
	gossip3messages "github.com/quorumcontrol/tupelo/gossip3/messages"
	gossip3remote "github.com/quorumcontrol/tupelo/gossip3/remote"
	gossip3types "github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/spf13/cobra"
)

var (
	BlsSignKeys  []*bls.SignKey
	EcdsaKeys    []*ecdsa.PrivateKey
	testnodePort int
)

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
		signer.Actor.Tell(&gossip3messages.StartGossip{})
		stopOnSignal(signer)
	},
}

func setupNotaryGroup(local *gossip3types.Signer, keys []*PublicKeySet) *gossip3types.NotaryGroup {
	if len(keys) == 0 {
		panic(fmt.Sprintf("no keys provided"))
	}

	group := gossip3types.NewNotaryGroup("hardcodedprivatekeysareunsafe")

	if local != nil {
		group.AddSigner(local)
	}

	for _, keySet := range keys {
		ecdsaBytes := hexutil.MustDecode(keySet.EcdsaHexPublicKey)
		if local != nil && bytes.Equal(crypto.FromECDSAPub(local.DstKey), ecdsaBytes) {
			continue
		}

		verKeyBytes := hexutil.MustDecode(keySet.BlsHexPublicKey)
		signer := gossip3types.NewRemoteSigner(crypto.ToECDSAPub(ecdsaBytes), bls.BytesToVerKey(verKeyBytes))
		if local != nil {
			signer.Actor = actor.NewPID(signer.ActorAddress(local.DstKey), "tupelo-"+signer.ID)
		}
		group.AddSigner(signer)
	}

	return group
}

func setupGossipNode(ctx context.Context, ecdsaKeyHex string, blsKeyHex string, namespace string, port int) *gossip3types.Signer {
	gossip3remote.Start()

	ecdsaKey, err := crypto.ToECDSA(hexutil.MustDecode(ecdsaKeyHex))
	if err != nil {
		panic("error fetching ecdsa key - set env variable TUPELO_NODE_ECDSA_KEY_HEX")
	}

	blsKey := bls.BytesToSignKey(hexutil.MustDecode(blsKeyHex))
	localSigner := gossip3types.NewLocalSigner(&ecdsaKey.PublicKey, blsKey)

	log.Info("starting up a test node")

	storagePath := configDir(namespace)
	os.MkdirAll(storagePath, 0700)

	commitPath := filepath.Join(storagePath, localSigner.ID+"-commit")
	currentPath := filepath.Join(storagePath, localSigner.ID+"-current")
	badgerCommit, err := storage.NewBadgerStorage(commitPath)
	if err != nil {
		panic(fmt.Sprintf("error creating storage: %v", err))
	}
	badgerCurrent, err := storage.NewBadgerStorage(currentPath)
	if err != nil {
		panic(fmt.Sprintf("error creating storage: %v", err))
	}

	p2pHost, err := p2p.NewLibP2PHost(ctx, ecdsaKey, port)
	if err != nil {
		panic("error setting up p2p host")
	}
	p2pHost.Bootstrap(p2p.BootstrapNodes())
	err = p2pHost.WaitForBootstrap(1, 60*time.Second)
	if err != nil {
		panic(fmt.Sprintf("error waiting for bootstrap: %v", err))
	}

	gossip3remote.NewRouter(p2pHost)

	group := setupNotaryGroup(localSigner, bootstrapPublicKeys)

	act, err := actor.SpawnNamed(gossip3actors.NewTupeloNodeProps(&gossip3actors.TupeloConfig{
		Self:              localSigner,
		NotaryGroup:       group,
		CommitStore:       badgerCommit,
		CurrentStateStore: badgerCurrent,
	}), syncerActorName(localSigner))
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}

	localSigner.Actor = act
	return localSigner
}

func stopOnSignal(signers ...*gossip3types.Signer) {
	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		fmt.Println(sig)
		for _, signer := range signers {
			log.Info("gracefully stopping signer")
			signer.Actor.GracefulStop()
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
