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

type HumanPublicKeySet struct {
	VerKeyHex  string
	DestKeyHex string
}

func (hpubset *HumanPublicKeySet) ToPublicKeySet() (pubset PublicKeySet, err error) {
	blsBits, err := hexutil.Decode(hpubset.VerKeyHex)
	if err != nil {
		return pubset, fmt.Errorf("error decoding verkey: %v", err)
	}
	ecdsaBits, err := hexutil.Decode(hpubset.DestKeyHex)
	if err != nil {
		return pubset, fmt.Errorf("error decoding destkey: %v", err)
	}

	ecdsaPub, err := crypto.UnmarshalPubkey(ecdsaBits)
	if err != nil {
		return pubset, fmt.Errorf("couldn't unmarshal ECDSA pub key: %v", err)
	}

	verKey := bls.BytesToVerKey(blsBits)

	return PublicKeySet{
		DestKey: ecdsaPub,
		VerKey:  verKey,
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
	Signers        []HumanPublicKeySet
	BootstrapNodes []string

	BoostrapOnly  bool
	TracingSystem string
}

func HumanConfigToConfig(hc HumanConfig) (*Config, error) {
	c := &Config{
		Namespace:      hc.Namespace,
		StoragePath:    hc.StoragePath,
		PublicIP:       hc.PublicIP,
		Port:           hc.Port,
		BootstrapNodes: hc.BootstrapNodes,
		BootstrapOnly:  hc.BoostrapOnly,
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

	signers := make([]PublicKeySet, len(hc.Signers))
	for i, humanPub := range hc.Signers {
		pub, err := humanPub.ToPublicKeySet()
		if err != nil {
			return nil, fmt.Errorf("error getting signer from human: %v", err)
		}
		signers[i] = pub
	}
	c.Signers = signers

	return c, nil
}

// TomlToConfig will load a config from a toml string
func TomlToConfig(tomlBytes string) (*Config, error) {
	var hc HumanConfig
	_, err := toml.Decode(tomlBytes, &hc)
	if err != nil {
		return nil, fmt.Errorf("error decoding toml: %v", err)
	}
	return HumanConfigToConfig(hc)
}
