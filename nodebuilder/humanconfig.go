package nodebuilder

import (
	"fmt"

	"github.com/BurntSushi/toml"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"

	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
)

type HumanPrivateKeySet struct {
	SignKeyHex string
	DestKeyHex string
}

func (hpks *HumanPrivateKeySet) ToPrivateKeySet() (*PrivateKeySet, error) {
	signKeyBytes, err := hexutil.Decode(hpks.SignKeyHex)
	if err != nil {
		return nil, fmt.Errorf("error decoding sign key: %v", err)
	}
	destKeyBytes, err := hexutil.Decode(hpks.DestKeyHex)
	if err != nil {
		return nil, fmt.Errorf("error decoding dest key: %v", err)
	}
	ecdsaPrivate, err := crypto.ToECDSA(destKeyBytes)
	if err != nil {
		return nil, fmt.Errorf("couldn't unmarshal ECDSA private key: %v", err)
	}
	signKey := bls.BytesToSignKey(signKeyBytes)
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

	NotaryGroup types.HumanConfig
	StoragePath string
	PublicIP    string
	Port        int

	PrivateKeySet  *HumanPrivateKeySet
	BootstrapNodes []string

	BootstrapOnly bool
	TracingSystem string
}

func HumanConfigToConfig(hc HumanConfig) (*Config, error) {
	c := &Config{
		Namespace:      hc.Namespace,
		StoragePath:    hc.StoragePath,
		PublicIP:       hc.PublicIP,
		Port:           hc.Port,
		BootstrapNodes: hc.BootstrapNodes,
		BootstrapOnly:  hc.BootstrapOnly,
	}

	ngConfig, err := types.HumanConfigToConfig(&hc.NotaryGroup)
	if err != nil {
		return nil, fmt.Errorf("error getting notary group config: %v", err)
	}
	c.NotaryGroupConfig = ngConfig

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

// TomlToConfig will load a config from a toml string
func TomlToConfig(tomlStr string) (*Config, error) {
	var hc HumanConfig
	_, err := toml.Decode(tomlStr, &hc)
	if err != nil {
		return nil, fmt.Errorf("error decoding toml: %v", err)
	}
	return HumanConfigToConfig(hc)
}
