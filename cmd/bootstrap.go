package cmd

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/quorumcontrol/tupelo/nodebuilder"

	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/middleware"
	"github.com/spf13/cobra"
)

var bootstrapNodePort int

type bootstrapperConfigurationRaw struct {
	EcdsaHex   string   `json:"ecdsaHex"`
	PeerAddrs  []string `json:"peerAddrs"`
	ExternalIp string   `json:"externalIp"`
}

type bootstrapperConfiguration struct {
	ecdsaKey   *ecdsa.PrivateKey
	peerAddrs  []string
	externalIp string
}

func (b bootstrapperConfiguration) PrivateKeySet() *nodebuilder.PrivateKeySet {
	return &nodebuilder.PrivateKeySet{
		DestKey: b.ecdsaKey,
	}
}

func (b bootstrapperConfiguration) BootstrapNodes() []string {
	return b.peerAddrs
}

func (b bootstrapperConfiguration) PublicIP() string {
	return b.externalIp
}

func decodeBootstrapperConfig(confB []byte) (bootstrapperConfiguration, error) {
	var config bootstrapperConfiguration
	var confRaw bootstrapperConfigurationRaw
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

	config.ecdsaKey = ecdsaKey
	config.peerAddrs = confRaw.PeerAddrs
	config.externalIp = confRaw.ExternalIp

	return config, nil
}

func loadBootstrapperConfig(ctx context.Context, cancel context.CancelFunc) (
	*bootstrapperConfiguration, error) {
	confB, err := readConfJson(ctx, cancel)
	if err == nil {
		config, err := decodeBootstrapperConfig(confB)
		if err != nil {
			return nil, err
		}
		middleware.Log.Infow("successfully loaded configuration from file")
		return &config, err
	}

	return nil, nil
}

var bootstrapNodeCmd = &cobra.Command{
	Use:   "bootstrap-node",
	Short: "Run a bootstrap node",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		config, err := loadBootstrapperConfig(ctx, cancel)
		if err != nil {
			panic(err)
		}

		c, err := nodebuilder.LegacyBootstrapConfig(configNamespace, bootstrapNodePort, config)
		if err != nil {
			panic(fmt.Errorf("error getting config: %v", err))
		}

		nb := &nodebuilder.NodeBuilder{Config: c}

		err = nb.Start(ctx)
		if err != nil {
			panic(fmt.Errorf("error starting bootstrap: %v", err))
		}

		if err := startPromServer(); err != nil {
			panic(err)
		}

		fmt.Println("Bootstrap node running at:")
		for _, addr := range nb.Host().Addresses() {
			fmt.Println(addr)
		}

		stopBootstrapperOnSignal(ctx)
	},
}

func stopBootstrapperOnSignal(ctx context.Context) {
	sigs := make(chan os.Signal, 1)
	done := make(chan struct{}, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		middleware.Log.Debugw("received signal", "signal", sig)
		done <- struct{}{}
	}()
	middleware.Log.Infow("awaiting signal...")

	select {
	case <-done:
		middleware.Log.Debugw("stopping due to receipt of signal")
	case <-ctx.Done():
		middleware.Log.Debugw("stopping due to canceling of context")
	}

	middleware.Log.Infow("exiting")
}

func init() {
	rootCmd.AddCommand(bootstrapNodeCmd)
	bootstrapNodeCmd.Flags().IntVarP(&bootstrapNodePort, "port", "p", 0, "what port to use (default random)")
}
