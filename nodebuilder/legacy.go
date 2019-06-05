package nodebuilder

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
)

const (
	publicKeyFile  = "public-keys.json"
	privateKeyFile = "private-keys.json"

	localConfigName  = "local-network"
	remoteConfigName = "remote-network"

	bootstrapKeyFile = "bootstrap-keys.json"
)

type LegacyPublicKeySet struct {
	BlsHexPublicKey   string `json:"blsHexPublicKey,omitempty"`
	EcdsaHexPublicKey string `json:"ecdsaHexPublicKey,omitempty"`
	PeerIDBase58Key   string `json:"peerIDBase58Key,omitempty"`
}

func (lpks *LegacyPublicKeySet) ToPublicKeySet() (*PublicKeySet, error) {
	blsBits := hexutil.MustDecode(lpks.BlsHexPublicKey)
	ecdsaBits := hexutil.MustDecode(lpks.EcdsaHexPublicKey)

	ecdsaPub, err := crypto.UnmarshalPubkey(ecdsaBits)
	if err != nil {
		return nil, fmt.Errorf("couldn't unmarshal ECDSA pub key: %v", err)
	}

	verKey := bls.BytesToVerKey(blsBits)

	return &PublicKeySet{
		DestKey: ecdsaPub,
		VerKey:  verKey,
	}, nil
}

// type LegacyPrivateKeySet struct {
// 	BlsHexPrivateKey   string `json:"blsHexPrivateKey,omitempty"`
// 	EcdsaHexPrivateKey string `json:"ecdsaHexPrivateKey,omitempty"`
// }

func readBootstrapKeys(namespace string, overridePath string) ([]*LegacyPublicKeySet, error) {
	var keySet []*LegacyPublicKeySet
	var err error

	if overridePath == "" {
		err = loadKeyFile(&keySet, configDir(namespace, remoteConfigName), bootstrapKeyFile)
	} else {
		err = loadKeyFile(keySet, overridePath, publicKeyFile)
	}

	return keySet, err
}

func LegacySignerConfig(namespace string, port int, enableElasticTracing, enableJaegerTracing bool, overrideKeysFile string) (*Config, error) {
	if enableElasticTracing && enableJaegerTracing {
		return nil, fmt.Errorf("only one tracing library may be used at once")
	}

	ecdsaKeyHex := os.Getenv("TUPELO_NODE_ECDSA_KEY_HEX")
	blsKeyHex := os.Getenv("TUPELO_NODE_BLS_KEY_HEX")

	ecdsaKey, err := crypto.ToECDSA(hexutil.MustDecode(ecdsaKeyHex))
	if err != nil {
		return nil, fmt.Errorf("error decoding ECDSA key (from $TUPELO_NODE_ECDSA_KEY_HEX)")
	}

	blsKey := bls.BytesToSignKey(hexutil.MustDecode(blsKeyHex))

	privateKeySet := &PrivateKeySet{
		DestKey: ecdsaKey,
		SignKey: blsKey,
	}

	legacyKeys, err := readBootstrapKeys(namespace, overrideKeysFile)
	if err != nil {
		return nil, fmt.Errorf("error getting bootstrap keys: %v", err)
	}

	signers := make([]PublicKeySet, len(legacyKeys))

	for i, legacy := range legacyKeys {
		pub, err := legacy.ToPublicKeySet()
		if err != nil {
			return nil, fmt.Errorf("error converting public keys: %v", err)
		}
		signers[i] = *pub
	}

	ngConfig := types.DefaultConfig()
	ngConfig.ID = "hardcodedprivatekeysareunsafe" // this is what it was

	c := &Config{
		Namespace:         namespace,
		NotaryGroupConfig: ngConfig,
		Signers:           signers,
		PrivateKeySet:     privateKeySet,
	}
	if enableElasticTracing {
		c.TracingSystem = ElasticTracing
	}
	if enableJaegerTracing { // we check above to make sure we're not overwriting
		c.TracingSystem = JaegerTracing
	}

	return c, nil
}

func loadBootstrapKeyFile(path string) ([]*LegacyPublicKeySet, error) {
	var keySet []*LegacyPublicKeySet

	file, err := ioutil.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("error reading path %v: %v", path, err)
	}

	err = unmarshalKeys(keySet, file)

	return keySet, err
}

func loadKeyFile(keySet interface{}, path string, name string) error {
	jsonBytes, err := readConfig(path, name)
	if err != nil {
		return fmt.Errorf("error loading key file: %v", err)
	}

	return unmarshalKeys(keySet, jsonBytes)
}

// func loadPublicKeyFile(path string) ([]*PublicKeySet, error) {
// 	var keySet []*PublicKeySet
// 	err := loadKeyFile(&keySet, path, publicKeyFile)

// 	return keySet, err
// }

// func loadPrivateKeyFile(path string) ([]*PrivateKeySet, error) {
// 	var keySet []*PrivateKeySet
// 	err := loadKeyFile(&keySet, path, privateKeyFile)

// 	return keySet, err
// }

func unmarshalKeys(keySet interface{}, bytes []byte) error {
	if bytes != nil {
		err := json.Unmarshal(bytes, keySet)
		if err != nil {
			return err
		}
	}

	return nil
}

func readConfig(path string, filename string) ([]byte, error) {
	_, err := os.Stat(filepath.Join(path, filename))
	if os.IsNotExist(err) {
		return nil, nil
	}

	return ioutil.ReadFile(filepath.Join(path, filename))
}
