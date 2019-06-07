package nodebuilder

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/quorumcontrol/tupelo-go-sdk/consensus"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"

	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
)

const (
	publicKeyFile  = "public-keys.json"
	privateKeyFile = "private-keys.json"

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

type LegacyPrivateKeySet struct {
	BlsHexPrivateKey   string `json:"blsHexPrivateKey,omitempty"`
	EcdsaHexPrivateKey string `json:"ecdsaHexPrivateKey,omitempty"`
}

func (lpks *LegacyPrivateKeySet) ToPrivateKeySet() (*PrivateKeySet, error) {
	blsBits := hexutil.MustDecode(lpks.BlsHexPrivateKey)
	ecdsaBits := hexutil.MustDecode(lpks.EcdsaHexPrivateKey)

	ecdsaPrivate, err := crypto.ToECDSA(ecdsaBits)
	if err != nil {
		return nil, fmt.Errorf("couldn't unmarshal ECDSA private key: %v", err)
	}

	return &PrivateKeySet{
		DestKey: ecdsaPrivate,
		SignKey: bls.BytesToSignKey(blsBits),
	}, nil
}

func readBootstrapKeys(namespace string, overridePath string) ([]*LegacyPublicKeySet, error) {
	var keySet []*LegacyPublicKeySet
	var err error

	if overridePath == "" {
		err = loadKeyFile(&keySet, configDir(namespace, remoteConfigName), bootstrapKeyFile)
	} else {
		err = loadKeyFile(&keySet, overridePath, publicKeyFile)
	}

	return keySet, err
}

func LegacyBootstrapConfig(namespace string, port int) (*Config, error) {
	ecdsaKeyHex := os.Getenv("TUPELO_NODE_ECDSA_KEY_HEX")
	ecdsaKey, err := crypto.ToECDSA(hexutil.MustDecode(ecdsaKeyHex))
	if err != nil {
		return nil, fmt.Errorf("error decoding ecdsa key: %s", err)
	}

	return &Config{
		Namespace: namespace,
		PrivateKeySet: &PrivateKeySet{
			DestKey: ecdsaKey,
		},
		BootstrapNodes: p2p.BootstrapNodes(),
		BootstrapOnly:  true,
		Port:           port,
	}, nil
}

func LegacyConfig(namespace string, port int, enableElasticTracing, enableJaegerTracing bool, overrideKeysFile string) (*Config, error) {
	if enableElasticTracing && enableJaegerTracing {
		return nil, fmt.Errorf("only one tracing library may be used at once")
	}

	var privateKeySet *PrivateKeySet
	ecdsaKeyHex, ok := os.LookupEnv("TUPELO_NODE_ECDSA_KEY_HEX")
	if ok {
		blsKeyHex := os.Getenv("TUPELO_NODE_BLS_KEY_HEX")
		ecdsaKey, err := crypto.ToECDSA(hexutil.MustDecode(ecdsaKeyHex))
		if err != nil {
			return nil, fmt.Errorf("error decoding ECDSA key (from $TUPELO_NODE_ECDSA_KEY_HEX)")
		}

		blsKey := bls.BytesToSignKey(hexutil.MustDecode(blsKeyHex))

		privateKeySet = &PrivateKeySet{
			DestKey: ecdsaKey,
			SignKey: blsKey,
		}
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
		BootstrapNodes:    p2p.BootstrapNodes(),
		StoragePath:       configDir(namespace, remoteConfigName),
	}
	if enableElasticTracing {
		c.TracingSystem = ElasticTracing
	}
	if enableJaegerTracing { // we check above to make sure we're not overwriting
		c.TracingSystem = JaegerTracing
	}

	return c, nil
}

func LegacyLocalNetwork(ctx context.Context, namespace string, nodeCount int) (*LocalNetwork, error) {
	legacyKeys, _, err := loadLocalKeys(namespace, nodeCount)
	if err != nil {
		return nil, fmt.Errorf("error generating node keys: %v", err)
	}

	keys := make([]*PrivateKeySet, len(legacyKeys))
	for i, legacyKey := range legacyKeys {
		privKey, err := legacyKey.ToPrivateKeySet()
		if err != nil {
			return nil, fmt.Errorf("error getting private keys: %v", err)
		}
		keys[i] = privKey
	}
	ngConfig := types.DefaultConfig()
	ngConfig.ID = "localnetwork"

	return NewLocalNetwork(ctx, namespace, keys, ngConfig)
}

func loadKeyFile(keySet interface{}, path string, name string) error {
	jsonBytes, err := readConfig(path, name)
	if err != nil {
		return fmt.Errorf("error loading key file: %v", err)
	}

	return unmarshalKeys(keySet, jsonBytes)
}

func loadPublicKeyFile(path string) ([]*LegacyPublicKeySet, error) {
	var keySet []*LegacyPublicKeySet
	err := loadKeyFile(&keySet, path, publicKeyFile)

	return keySet, err
}

func loadPrivateKeyFile(path string) ([]*LegacyPrivateKeySet, error) {
	var keySet []*LegacyPrivateKeySet
	err := loadKeyFile(&keySet, path, privateKeyFile)

	return keySet, err
}

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

func loadLocalKeys(namespace string, num int) ([]*LegacyPrivateKeySet, []*LegacyPublicKeySet, error) {
	privateKeys, err := loadPrivateKeyFile(configDir(namespace, localConfigName))
	if err != nil {
		return nil, nil, fmt.Errorf("error loading private keys: %v", err)
	}

	publicKeys, err := loadPublicKeyFile(configDir(namespace, localConfigName))
	if err != nil {
		return nil, nil, fmt.Errorf("error loading public keys: %v", err)
	}

	savedKeyCount := len(privateKeys)
	if savedKeyCount < num {
		extraPrivateKeys, extraPublicKeys, err := generateKeySets(num - savedKeyCount)
		if err != nil {
			return nil, nil, fmt.Errorf("error generating extra node keys: %v", err)
		}

		combinedPrivateKeys := append(privateKeys, extraPrivateKeys...)
		combinedPublicKeys := append(publicKeys, extraPublicKeys...)

		err = writeJSONKeys(combinedPrivateKeys, combinedPublicKeys, configDir(namespace, localConfigName))
		if err != nil {
			return nil, nil, fmt.Errorf("error writing extra node keys: %v", err)
		}

		return combinedPrivateKeys, combinedPublicKeys, nil
	} else if savedKeyCount > num {
		return privateKeys[:num], publicKeys[:num], nil
	} else {
		return privateKeys, publicKeys, nil
	}
}

func generateKeySets(numberOfKeys int) (privateKeys []*LegacyPrivateKeySet, publicKeys []*LegacyPublicKeySet, err error) {
	for i := 1; i <= numberOfKeys; i++ {
		blsKey, err := bls.NewSignKey()
		if err != nil {
			return nil, nil, err
		}
		ecdsaKey, err := crypto.GenerateKey()
		if err != nil {
			return nil, nil, err
		}
		peerID, err := p2p.PeerIDFromPublicKey(&ecdsaKey.PublicKey)
		if err != nil {
			return nil, nil, err
		}

		privateKeys = append(privateKeys, &LegacyPrivateKeySet{
			BlsHexPrivateKey:   hexutil.Encode(blsKey.Bytes()),
			EcdsaHexPrivateKey: hexutil.Encode(crypto.FromECDSA(ecdsaKey)),
		})

		publicKeys = append(publicKeys, &LegacyPublicKeySet{
			BlsHexPublicKey:   hexutil.Encode(consensus.BlsKeyToPublicKey(blsKey.MustVerKey()).PublicKey),
			EcdsaHexPublicKey: hexutil.Encode(consensus.EcdsaToPublicKey(&ecdsaKey.PublicKey).PublicKey),
			PeerIDBase58Key:   peerID.Pretty(),
		})
	}

	return privateKeys, publicKeys, err
}

func writeJSONKeys(privateKeys []*LegacyPrivateKeySet, publicKeys []*LegacyPublicKeySet, path string) error {
	publicKeyJson, err := json.Marshal(publicKeys)
	if err != nil {
		return fmt.Errorf("Error marshaling public keys: %v", err)
	}

	err = writeFile(path, publicKeyFile, publicKeyJson)
	if err != nil {
		return fmt.Errorf("error writing public keys: %v", err)
	}

	privateKeyJson, err := json.Marshal(privateKeys)
	if err != nil {
		return fmt.Errorf("Error marshaling private keys: %v", err)
	}

	err = writeFile(path, privateKeyFile, privateKeyJson)
	if err != nil {
		return fmt.Errorf("error writing private keys: %v", err)
	}

	return nil
}

func writeFile(parentDir string, filename string, data []byte) error {
	err := os.MkdirAll(parentDir, 0755)
	if err != nil {
		return fmt.Errorf("error creating directory: %v", err)
	}

	return ioutil.WriteFile(filepath.Join(parentDir, filename), data, 0644)
}
