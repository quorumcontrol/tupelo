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
	"time"

	"github.com/quorumcontrol/tupelo/nodebuilder"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	gossip3types "github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
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
	Short: "Run a tupelo node",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		remote.Start()

		c, err := nodebuilder.LegacyConfig(configNamespace, testnodePort, enableElasticTracing, enableJaegerTracing, overrideKeysFile)
		if err != nil {
			panic(fmt.Errorf("error getting legacy config: %v", err))
		}
		nb := &nodebuilder.NodeBuilder{Config: c}
		err = nb.Start(ctx)
		if err != nil {
			panic(fmt.Errorf("error starting: %v", err))
		}
		err = nb.Host().WaitForBootstrap(len(c.Signers)/2, 30*time.Second)
		if err != nil {
			panic(fmt.Errorf("error starting: %v", err))
		}
		fmt.Printf("started signer host %s on %v\n", nb.Host().Identity(), nb.Host().Addresses())
		select {}
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

func init() {
	rootCmd.AddCommand(testnodeCmd)
	testnodeCmd.Flags().IntVarP(&testnodePort, "port", "p", 0, "what port will the node listen on")
	testnodeCmd.Flags().BoolVar(&enableJaegerTracing, "jaeger-tracing", false, "enable jaeger tracing")
	testnodeCmd.Flags().BoolVar(&enableElasticTracing, "elastic-tracing", false, "enable elastic tracing")
}
