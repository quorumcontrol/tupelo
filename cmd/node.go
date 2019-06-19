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
	"encoding/json"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	logging "github.com/ipfs/go-log"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"

	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	"github.com/quorumcontrol/tupelo/nodebuilder"

	"github.com/spf13/cobra"
)

var (
	testnodePort int

	enableJaegerTracing  bool
	enableElasticTracing bool
)

type signerConfigurationRaw struct {
	EcdsaHex          string                           `json:"ecdsaHex"`
	BlsHex            string                           `json:"blsHex"`
	PubKeySets        []nodebuilder.LegacyPublicKeySet `json:"pubKeySets"`
	ExternalIp        string                           `json:"externalIp"`
	BootstrapperAddrs []string                         `json:"bootstrapperAddrs"`
}

type signerConfiguration struct {
	ecdsaKey          *ecdsa.PrivateKey
	blsKey            *bls.SignKey
	pubKeySets        []nodebuilder.PublicKeySet
	externalIp        string
	bootstrapperAddrs []string
}

func (s signerConfiguration) PrivateKeySet() *nodebuilder.PrivateKeySet {
	return &nodebuilder.PrivateKeySet{
		SignKey: s.blsKey,
		DestKey: s.ecdsaKey,
	}
}

func (s signerConfiguration) Signers() []nodebuilder.PublicKeySet {
	return s.pubKeySets
}

func (s signerConfiguration) BootstrapNodes() []string {
	return s.bootstrapperAddrs
}

func (s signerConfiguration) PublicIP() string {
	return s.externalIp
}

func decodeSignerConfig(confB []byte) (signerConfiguration, error) {
	var config signerConfiguration
	var confRaw signerConfigurationRaw
	if err := json.Unmarshal(confB, &confRaw); err != nil {
		return config, err
	}

	ecdsaB, err := hexutil.Decode(confRaw.EcdsaHex)
	if err != nil {
		return config, fmt.Errorf("error decoding ECDSA key: %s", err)
	}
	ecdsaKey, err := crypto.ToECDSA(ecdsaB)
	if err != nil {
		return config, fmt.Errorf("error decoding ECDSA key: %s", err)
	}

	blsB, err := hexutil.Decode(confRaw.BlsHex)
	if err != nil {
		return config, fmt.Errorf("error decoding BLS key: %s", err)
	}
	blsKey := bls.BytesToSignKey(blsB)

	config.ecdsaKey = ecdsaKey
	config.blsKey = blsKey
	config.pubKeySets = make([]nodebuilder.PublicKeySet, len(confRaw.PubKeySets))
	for i, pubKeySet := range confRaw.PubKeySets {
		pks, err := pubKeySet.ToPublicKeySet()
		if err != nil {
			return config, err
		}
		config.pubKeySets[i] = *pks
	}
	config.bootstrapperAddrs = confRaw.BootstrapperAddrs
	config.externalIp = confRaw.ExternalIp
	if config.externalIp == "" {
		panic("external IP must be set")
	}

	return config, nil
}

func loadSignerConfig() (*signerConfiguration, error) {
	confB, err := readConfJson()
	if err == nil {
		config, err := decodeSignerConfig(confB)
		if err != nil {
			return nil, err
		}
		middleware.Log.Infow("successfully loaded configuration from file")
		return &config, err
	}

	return nil, nil
}

// testnodeCmd represents the test-node command
var testnodeCmd = &cobra.Command{
	Use:   "test-node [index of key]",
	Short: "Run a tupelo node",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := logging.SetLogLevel("gossip", "ERROR"); err != nil {
			log.Error("failed to set log level of 'gossip'", "err", err)
		}

		remote.Start()

		config, err := loadSignerConfig()
		if err != nil {
			panic(err)
		}
		c, err := nodebuilder.LegacyConfig(configNamespace, testnodePort, enableElasticTracing,
			enableJaegerTracing, overrideKeysFile, config)
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

func init() {
	rootCmd.AddCommand(testnodeCmd)
	testnodeCmd.Flags().IntVarP(&testnodePort, "port", "p", 0, "what port will the node listen on")
	testnodeCmd.Flags().BoolVar(&enableJaegerTracing, "jaeger-tracing", false, "enable jaeger tracing")
	testnodeCmd.Flags().BoolVar(&enableElasticTracing, "elastic-tracing", false, "enable elastic tracing")
}
