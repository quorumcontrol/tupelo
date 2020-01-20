package nodebuilder

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/BurntSushi/toml"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	g3types "github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"

	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/types"
)

type HumanPrivateKeySet struct {
	SignKeyHex string
	DestKeyHex string
}

func (hpks *HumanPrivateKeySet) ToPrivateKeySet() (*PrivateKeySet, error) {
	var (
		signKeyBytes []byte
		signKey      *bls.SignKey
		err          error
	)

	if len(hpks.SignKeyHex) > 0 {
		signKeyBytes, err = hexutil.Decode(hpks.SignKeyHex)
		if err != nil {
			return nil, fmt.Errorf("error decoding sign key: %v", err)
		}
		signKey = bls.BytesToSignKey(signKeyBytes)
	}

	destKeyBytes, err := hexutil.Decode(hpks.DestKeyHex)
	if err != nil {
		return nil, fmt.Errorf("error decoding dest key: %v", err)
	}

	ecdsaPrivate, err := crypto.ToECDSA(destKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("couldn't unmarshal ECDSA private key: %v", err)
	}

	return &PrivateKeySet{
		SignKey: signKey,
		DestKey: ecdsaPrivate,
	}, nil
}

// HumanConfig is used for parsing an ondisk configuration into the application-used Config
// struct. At the time of this comment, it also uses the types.HumanConfig for notary groups
// defined in the tupelo-go-sdk as well.
type HumanConfig struct {
	Namespace string

	NotaryGroupConfig        string
	Gossip3NotaryGroupConfig string
	StoragePath              string
	PublicIP                 string
	Port                     int
	WebsocketPort            int

	PrivateKeySet *HumanPrivateKeySet

	BootstrapOnly bool
	TracingSystem string
}

func HumanConfigToConfig(hc HumanConfig) (*Config, error) {
	c := &Config{
		Namespace:     hc.Namespace,
		StoragePath:   hc.StoragePath,
		PublicIP:      hc.PublicIP,
		Port:          hc.Port,
		WebsocketPort: hc.WebsocketPort,
		BootstrapOnly: hc.BootstrapOnly,
	}

	tomlBits, err := ioutil.ReadFile(hc.NotaryGroupConfig)
	if err != nil {
		return nil, fmt.Errorf("error reading %v", err)
	}

	ngConfig, err := types.TomlToConfig(string(tomlBits))
	if err != nil {
		return nil, fmt.Errorf("error loading notary group config: %v", err)
	}
	c.NotaryGroupConfig = ngConfig

	if hc.Gossip3NotaryGroupConfig != "" {
		g3TomlBits, err := ioutil.ReadFile(hc.Gossip3NotaryGroupConfig)
		if err != nil {
			return nil, fmt.Errorf("error reading %v", err)
		}

		g3NgConfig, err := g3types.TomlToConfig(string(g3TomlBits))
		if err != nil {
			return nil, fmt.Errorf("error loading gossip3 notary group config: %v", err)
		}
		c.Gossip3NotaryGroupConfig = g3NgConfig
	}

	c.BootstrapNodes = ngConfig.BootstrapAddresses

	switch hc.TracingSystem {
	case "":
		// do nothing
	case "jaeger":
		c.TracingSystem = JaegerTracing
	case "elastic":
		c.TracingSystem = ElasticTracing
	default:
		return nil, fmt.Errorf("only 'jaeger' and 'elastic' are supported for tracing")
	}

	if hc.PrivateKeySet != nil {
		privSet, err := hc.PrivateKeySet.ToPrivateKeySet()
		if err != nil {
			return nil, fmt.Errorf("error getting private keys: %v", err)
		}
		c.PrivateKeySet = privSet
	}

	return c, nil
}

// TomlToConfig will load a config from a path to a toml file
func TomlToConfig(path string) (*Config, error) {
	tomlBits, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading %v", err)
	}

	var hc HumanConfig
	_, err = toml.Decode(string(tomlBits), &hc)
	if err != nil {
		return nil, fmt.Errorf("error decoding toml: %v", err)
	}

	if hc.NotaryGroupConfig == "" {
		return nil, fmt.Errorf("missing notary group config path")
	}

	if !filepath.IsAbs(hc.NotaryGroupConfig) {
		relPath := hc.NotaryGroupConfig
		hc.NotaryGroupConfig = filepath.Join(filepath.Dir(path), relPath)
	}

	if hc.Gossip3NotaryGroupConfig != "" && !filepath.IsAbs(hc.Gossip3NotaryGroupConfig) {
		hc.Gossip3NotaryGroupConfig = filepath.Join(filepath.Dir(path), hc.Gossip3NotaryGroupConfig)
	}

	return HumanConfigToConfig(hc)
}
