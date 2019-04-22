package cmd

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"

	"github.com/quorumcontrol/tupelo/nodebuilder"

	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/middleware"
	"github.com/spf13/cobra"
)

var bootstrapNodePort int

func findBootstrapperPeers(host p2p.Node) ([]string, error) {
	middleware.Log.Debugw("detecting peer addresses via environment...")
	anAddr := host.Addresses()[0].String()
	keySlice := strings.Split(anAddr, "/")
	key := keySlice[len(keySlice)-1]
	bootstrapNodes := p2p.BootstrapNodes()
	peers := []string{}
	for _, nodeAddr := range bootstrapNodes {
		if !strings.Contains(nodeAddr, key) {
			peers = append(peers, nodeAddr)
		}
	}

	middleware.Log.Debugw("found peers", "peers", peers)

	return peers, nil
}

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

func loadBootstrapperConfig() (bootstrapperConfiguration, error) {
	var config bootstrapperConfiguration
	confB, err := readConfJson()
	if err == nil {
		config, err = decodeBootstrapperConfig(confB)
		if err != nil {
			return config, err
		}
		middleware.Log.Infow("successfully loaded configuration from file")
	} else {
		middleware.Log.Infow("loading configuration from environment")
		varName := "TUPELO_NODE_ECDSA_KEY_HEX"
		ecdsaHex := os.Getenv(varName)
		ecdsaBytes, err := hexutil.Decode(ecdsaHex)
		if err != nil {
			return config, fmt.Errorf("error decoding ECDSA key from $%s: %s", varName, err)
		}
		ecdsaKey, err := crypto.ToECDSA(ecdsaBytes)
		if err != nil {
			return config, fmt.Errorf("error decoding ecdsa key from $%s: %s", varName, err)
		}

		extIp := ""
		if pip, ok := os.LookupEnv("TUPELO_PUBLIC_IP"); ok {
			extIp = pip
			middleware.Log.Infow("got external IP from $TUPELO_PUBLIC_IP", "externalIp", extIp)
		}

		config.ecdsaKey = ecdsaKey
		config.externalIp = extIp
	}

	return config, nil
}

var bootstrapNodeCmd = &cobra.Command{
	Use:   "bootstrap-node",
	Short: "Run a bootstrap node",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		config, err := loadBootstrapperConfig()
		if err != nil {
			panic(err)
		}

		ctx := context.WithCancel(context.Background())
		defer cancel()
		
		cm := connmgr.NewConnManager(4915, 7372, 30*time.Second)

		c, err := nodebuilder.LegacyBootstrapConfig(configNamespace, bootstrapNodePort)
		if err != nil {
			panic(fmt.Errorf("error getting config: %v", err))
		}

		nb := &nodebuilder.NodeBuilder{Config: c}

		err = nb.Start(ctx)
		if err != nil {
			panic(fmt.Errorf("error starting bootstrap: %v", err))
		}

		fmt.Println("Bootstrap node running at:")
		for _, addr := range nb.Host().Addresses() {
			fmt.Println(addr)
		}
		select {}
	},
}

func init() {
	rootCmd.AddCommand(bootstrapNodeCmd)
	bootstrapNodeCmd.Flags().IntVarP(&bootstrapNodePort, "port", "p", 0, "what port to use (default random)")
}
