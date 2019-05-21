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
	"syscall"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	gossip3remote "github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	gossip3types "github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
	"github.com/quorumcontrol/tupelo-go-sdk/tracing"
	gossip3actors "github.com/quorumcontrol/tupelo/gossip3/actors"
	"github.com/spf13/cobra"
)

var (
	BlsSignKeys  []*bls.SignKey
	EcdsaKeys    []*ecdsa.PrivateKey
	testnodePort int

	enableJaegerTracing  bool
	enableElasticTracing bool
)

// testnodeCmd represents the test-node command
var testnodeCmd = &cobra.Command{
	Use:   "test-node [index of key]",
	Short: "Run a testnet node with hardcoded (insecure) keys",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := logging.SetLogLevel("gossip", "ERROR"); err != nil {
			log.Error("failed to set log level of 'gossip'", "err", err)
		}
		ecdsaKeyHex := os.Getenv("TUPELO_NODE_ECDSA_KEY_HEX")
		blsKeyHex := os.Getenv("TUPELO_NODE_BLS_KEY_HEX")
		signer := setupGossipNode(ctx, ecdsaKeyHex, blsKeyHex, "distributed-network", testnodePort)
		if enableElasticTracing && enableJaegerTracing {
			panic("only one tracing library may be used at once")
		}
		if enableJaegerTracing {
			tracing.StartJaeger("signer-" + signer.ID)
		}
		if enableElasticTracing {
			tracing.StartElastic()
		}
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
		ecdsaPub, err := crypto.UnmarshalPubkey(ecdsaBytes)
		if err != nil {
			panic("couldn't unmarshal ECDSA pub key")
		}
		signer := gossip3types.NewRemoteSigner(ecdsaPub, bls.BytesToVerKey(verKeyBytes))
		if local != nil {
			signer.Actor = actor.NewPID(signer.ActorAddress(local.DstKey), syncerActorName(signer))
		}
		group.AddSigner(signer)
	}

	return group
}

func p2pNodeWithOpts(ctx context.Context, ecdsaKey *ecdsa.PrivateKey, port int, addlOpts ...p2p.Option) (p2p.Node, error) {
	opts := []p2p.Option{
		p2p.WithKey(ecdsaKey),
		p2p.WithDiscoveryNamespaces("tupelo-transaction-gossipers"),
		p2p.WithListenIP("0.0.0.0", port),
	}
	if hostIP, ok := os.LookupEnv("TUPELO_PUBLIC_IP"); ok {
		opts = append(opts, p2p.WithExternalIP(hostIP, port))
	}
	return p2p.NewHostFromOptions(ctx, append(opts, addlOpts...)...)
}

func setupGossipNode(ctx context.Context, ecdsaKeyHex string, blsKeyHex string, namespace string, port int) *gossip3types.Signer {
	gossip3remote.Start()

	ecdsaKey, err := crypto.ToECDSA(hexutil.MustDecode(ecdsaKeyHex))
	if err != nil {
		panic("error decoding ECDSA key (from $TUPELO_NODE_ECDSA_KEY_HEX)")
	}

	blsKey := bls.BytesToSignKey(hexutil.MustDecode(blsKeyHex))
	localSigner := gossip3types.NewLocalSigner(&ecdsaKey.PublicKey, blsKey)

	log.Info("starting up a test node")

	storagePath := configDir(namespace)
	currentPath := signerCurrentPath(storagePath, localSigner)

	badgerCurrent, err := storage.NewBadgerStorage(currentPath)
	if err != nil {
		panic(fmt.Sprintf("error creating storage: %v", err))
	}

	group := setupNotaryGroup(localSigner, bootstrapPublicKeys)

	cm := connmgr.NewConnManager(len(group.Signers)*2, 900, 20*time.Second)
	for _, s := range group.Signers {
		id, err := p2p.PeerFromEcdsaKey(s.DstKey)
		if err != nil {
			panic(fmt.Sprintf("error getting peer from ecdsa key: %v", err))
		}
		cm.Protect(id, "signer")
	}

	p2pHost, err := p2pNodeWithOpts(ctx, ecdsaKey, port, p2p.WithLibp2pOptions(libp2p.ConnectionManager(cm)))
	if err != nil {
		panic("error setting up p2p host")
	}
	if _, err = p2pHost.Bootstrap(p2p.BootstrapNodes()); err != nil {
		panic(fmt.Sprintf("failed to bootstrap: %s", err))
	}

	// wait until we connect to half the network
	err = p2pHost.WaitForBootstrap(1+len(group.Signers)/2, 60*time.Second)
	if err != nil {
		panic(fmt.Sprintf("error waiting for bootstrap: %v", err))
	}

	gossip3remote.NewRouter(p2pHost)

	act, err := actor.SpawnNamed(gossip3actors.NewTupeloNodeProps(&gossip3actors.TupeloConfig{
		Self:              localSigner,
		NotaryGroup:       group,
		CurrentStateStore: badgerCurrent,
		PubSubSystem:      remote.NewNetworkPubSub(p2pHost),
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
			err := actor.EmptyRootContext.PoisonFuture(signer.Actor).Wait()
			if err != nil {
				log.Error(fmt.Sprintf("signer failed to stop gracefully: %v", err))
			}
		}
		done <- true
	}()
	fmt.Println("awaiting signal")
	<-done
	if enableJaegerTracing {
		tracing.StopJaeger()
	}
	fmt.Println("exiting")
}

func init() {
	rootCmd.AddCommand(testnodeCmd)
	testnodeCmd.Flags().IntVarP(&testnodePort, "port", "p", 0, "what port will the node listen on")
	testnodeCmd.Flags().BoolVar(&enableJaegerTracing, "jaeger-tracing", false, "enable jaeger tracing")
	testnodeCmd.Flags().BoolVar(&enableElasticTracing, "elastic-tracing", false, "enable elastic tracing")
}