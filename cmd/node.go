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
	"encoding/json"
	"fmt"
	"time"

	"github.com/pkg/errors"

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
	EcdsaHex          string          `json:"ecdsaHex"`
	BlsHex            string          `json:"blsHex"`
	PubKeySets        []*PublicKeySet `json:"pubKeySets"`
	ExternalIp        string          `json:"externalIp"`
	BootstrapperAddrs []string        `json:"bootstrapperAddrs"`
}

type signerConfiguration struct {
	ecdsaKey          *ecdsa.PrivateKey
	blsKey            *bls.SignKey
	pubKeySets        []*PublicKeySet
	externalIp        string
	bootstrapperAddrs []string
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
	config.pubKeySets = confRaw.PubKeySets
	config.bootstrapperAddrs = confRaw.BootstrapperAddrs
	config.externalIp = confRaw.ExternalIp

	return config, nil
}

func loadSignerConfig() (signerConfiguration, error) {
	var config signerConfiguration
	confB, err := readConfJson()
	if err == nil {
		config, err = decodeSignerConfig(confB)
		if err != nil {
			return config, err
		}
		middleware.Log.Infow("successfully loaded configuration from file")
	} else {
		middleware.Log.Infow("loading configuration from environment")
		sourceEcdsa := "TUPELO_NODE_ECDSA_KEY_HEX"
		ecdsaKeyHex := os.Getenv(sourceEcdsa)
		ecdsaKeyB, err := hexutil.Decode(ecdsaKeyHex)
		if err != nil {
			return config, fmt.Errorf("error decoding ECDSA key (from $%s)", sourceEcdsa)
		}
		ecdsaKey, err := crypto.ToECDSA(ecdsaKeyB)
		if err != nil {
			return config, fmt.Errorf("error decoding ECDSA key (from $%s)", sourceEcdsa)
		}

		sourceBls := "TUPELO_NODE_BLS_KEY_HEX"
		blsKeyHex := os.Getenv(sourceBls)
		blsKeyB, err := hexutil.Decode(blsKeyHex)
		if err != nil {
			return config, fmt.Errorf("error decoding BLS key (from $%s)", sourceBls)
		}
		blsKey := bls.BytesToSignKey(blsKeyB)

		extIp := ""
		if pip, ok := os.LookupEnv("TUPELO_PUBLIC_IP"); ok {
			extIp = pip
			middleware.Log.Infow("got external IP from $TUPELO_PUBLIC_IP",
				"externalIp", extIp)
		}

		config.ecdsaKey = ecdsaKey
		config.blsKey = blsKey
		config.pubKeySets = bootstrapPublicKeys
		config.externalIp = extIp
		config.bootstrapperAddrs = p2p.BootstrapNodes()
	}

	return config, nil
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

		config, err := loadSignerConfig()
		if err != nil {
			panic(err)
		}
		signer, err := setupGossipNode(ctx, config, "distributed-network", testnodePort)
		if err != nil {
			panic(err)
		}
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

func setupNotaryGroup(local *gossip3types.Signer, pubKeySets []*PublicKeySet) (
	*gossip3types.NotaryGroup, error) {
	group := gossip3types.NewNotaryGroup("hardcodedprivatekeysareunsafe")

	if local != nil {
		group.AddSigner(local)
	}

	for _, keySet := range pubKeySets {
		ecdsaBytes := hexutil.MustDecode(keySet.EcdsaHexPublicKey)
		if local != nil && bytes.Equal(crypto.FromECDSAPub(local.DstKey), ecdsaBytes) {
			continue
		}

		verKeyBytes := hexutil.MustDecode(keySet.BlsHexPublicKey)
		ecdsaPub, err := crypto.UnmarshalPubkey(ecdsaBytes)
		if err != nil {
			return nil, errors.Wrapf(err, "couldn't unmarshal ECDSA pub key")
		}
		signer := gossip3types.NewRemoteSigner(ecdsaPub, bls.BytesToVerKey(verKeyBytes))
		if local != nil {
			signer.Actor = actor.NewPID(signer.ActorAddress(local.DstKey), syncerActorName(signer))
		}
		group.AddSigner(signer)
	}

	return group, nil
}

func p2pNodeWithOpts(ctx context.Context, ecdsaKey *ecdsa.PrivateKey, port int,
	externalIp string, addlOpts ...p2p.Option) (p2p.Node, error) {
	opts := []p2p.Option{
		p2p.WithKey(ecdsaKey),
		p2p.WithDiscoveryNamespaces("tupelo-transaction-gossipers"),
		p2p.WithListenIP("0.0.0.0", port),
	}
	if externalIp != "" {
		opts = append(opts, p2p.WithExternalIP(externalIp, port))
	}
	return p2p.NewHostFromOptions(ctx, append(opts, addlOpts...)...)
}

func setupGossipNode(ctx context.Context, config signerConfiguration, namespace string,
	port int) (*gossip3types.Signer, error) {
	log.Info("starting up a gossip node")

	gossip3remote.Start()

	localSigner := gossip3types.NewLocalSigner(&config.ecdsaKey.PublicKey, config.blsKey)

	storagePath := configDir(namespace)
	currentPath := signerCurrentPath(storagePath, localSigner)

	badgerCurrent, err := storage.NewBadgerStorage(currentPath)
	if err != nil {
		return nil, err
	}

	group, err := setupNotaryGroup(localSigner, config.pubKeySets)
	if err != nil {
		return nil, err
	}

	cm := connmgr.NewConnManager(len(group.Signers)*2, 900, 20*time.Second)
	for _, s := range group.Signers {
		id, err := p2p.PeerFromEcdsaKey(s.DstKey)
		if err != nil {
			return nil, err
		}
		cm.Protect(id, "signer")
	}

	p2pHost, err := p2pNodeWithOpts(ctx, config.ecdsaKey, port, config.externalIp,
		p2p.WithLibp2pOptions(libp2p.ConnectionManager(cm)))
	if err != nil {
		return nil, err
	}

	if _, err = p2pHost.Bootstrap(config.bootstrapperAddrs); err != nil {
		return nil, err
	}

	// wait until we connect to half the network
	if err := p2pHost.WaitForBootstrap(1+len(group.Signers)/2, 60*time.Second); err != nil {
		return nil, err
	}

	gossip3remote.NewRouter(p2pHost)

	act, err := actor.SpawnNamed(gossip3actors.NewTupeloNodeProps(&gossip3actors.TupeloConfig{
		Self:              localSigner,
		NotaryGroup:       group,
		CurrentStateStore: badgerCurrent,
		PubSubSystem:      remote.NewNetworkPubSub(p2pHost),
	}), syncerActorName(localSigner))
	if err != nil {
		return nil, err
	}

	localSigner.Actor = act
	return localSigner, nil
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
