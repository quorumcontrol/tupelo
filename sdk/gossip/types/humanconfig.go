package types

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/quorumcontrol/messages/v2/build/go/config"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/quorumcontrol/tupelo/sdk/bls"

	"github.com/BurntSushi/toml"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/messages/v2/build/go/transactions"
	"github.com/quorumcontrol/tupelo/sdk/consensus"
)

func init() {
	for enum, fn := range consensus.DefaultTransactors {
		mustRegisterTransactor(transactions.Transaction_Type_name[int32(enum)], fn)
	}
}

// transactorRegistry allows for registering your functions to human-readable srings
var transactorRegistry = make(map[string]chaintree.TransactorFunc)

// RegisterTransactor is used to make transactors available for the human-readable configs
func RegisterTransactor(name string, fn chaintree.TransactorFunc) error {
	_, ok := transactions.Transaction_Type_value[name]
	if !ok {
		return fmt.Errorf("error: you must specify a name that is specified in transactions protobufs")
	}
	_, ok = transactorRegistry[name]
	if ok {
		return fmt.Errorf("error: %s already exists in the transactor registry", name)
	}
	transactorRegistry[name] = fn
	return nil
}

func mustRegisterTransactor(name string, fn chaintree.TransactorFunc) {
	err := RegisterTransactor(name, fn)
	if err != nil {
		panic(err)
	}
}

// validatorGeneratorRegistry is used for human-readable configs to specify what validators
// should be used for a notary group.
var validatorGeneratorRegistry = make(map[string]ValidatorGenerator)

// RegisterValidatorGenerator registers your validator generator with a human-readable name
// so that it can be specified in the on-disk configs.
func RegisterValidatorGenerator(name string, fn ValidatorGenerator) error {
	_, ok := validatorGeneratorRegistry[name]
	if ok {
		return fmt.Errorf("error: %s already exists in the validator registry", name)
	}
	validatorGeneratorRegistry[name] = fn
	return nil
}

func mustRegisterValidatorGenerator(name string, fn ValidatorGenerator) {
	err := RegisterValidatorGenerator(name, fn)
	if err != nil {
		panic(err)
	}
}

type PublicKeySet struct {
	VerKey  *bls.VerKey
	DestKey *ecdsa.PublicKey
}

type TomlPublicKeySet struct {
	VerKeyHex  string
	DestKeyHex string
}

// converts the toml hex-based keys to the protobuf based byte slice
func (tpubset *TomlPublicKeySet) toConfigPublicKeySet() (pubset *config.PublicKeySet, err error) {
	blsBits, err := hexutil.Decode(tpubset.VerKeyHex)
	if err != nil {
		return pubset, fmt.Errorf("error decoding verkey: %v", err)
	}
	ecdsaBits, err := hexutil.Decode(tpubset.DestKeyHex)
	if err != nil {
		return pubset, fmt.Errorf("error decoding destkey: %v", err)
	}

	return &config.PublicKeySet{
		DestKey: ecdsaBits,
		VerKey:  blsBits,
	}, nil
}

type TomlConfig struct {
	*config.NotaryGroup
	Signers []TomlPublicKeySet
}

// converts the toml config to the standard protobuf config
// in the messages repo. the only difference is that in toml
// the signer keys are saved as hex instead of byte slices.
func (tc *TomlConfig) toPBConfig() (*config.NotaryGroup, error) {
	ngConfig := tc.NotaryGroup
	if ngConfig == nil {
		return nil, fmt.Errorf("no notary group config")
	}
	ngConfig.Signers = make([]*config.PublicKeySet, len(tc.Signers))
	for i, tomlPubKeySet := range tc.Signers {
		c, err := tomlPubKeySet.toConfigPublicKeySet()
		if err != nil {
			return nil, fmt.Errorf("error converting hex to bytes: %v", err)
		}
		ngConfig.Signers[i] = c
	}
	return ngConfig, nil
}

func bytesToKeys(pubSet *config.PublicKeySet) (retPub PublicKeySet, err error) {
	ecdsaPub, err := crypto.UnmarshalPubkey(pubSet.DestKey)
	if err != nil {
		return retPub, fmt.Errorf("couldn't unmarshal ECDSA pub key: %v", err)
	}

	verKey := bls.BytesToVerKey(pubSet.VerKey)
	return PublicKeySet{
		VerKey:  verKey,
		DestKey: ecdsaPub,
	}, nil
}

func HumanConfigToConfig(hc *config.NotaryGroup) (*Config, error) {
	defaults := DefaultConfig()
	c := &Config{
		ID:                 hc.Id,
		TransactionToken:   hc.TransactionToken,
		BurnAmount:         hc.BurnAmount,
		TransactionTopic:   hc.TransactionTopic,
		CommitTopic:        hc.CommitTopic,
		BootstrapAddresses: hc.BootstrapAddresses,
	}

	if c.ID == "" {
		return nil, fmt.Errorf("error ID cannot be nil")
	}

	if c.TransactionToken == "" {
		c.TransactionToken = defaults.TransactionToken // at this moment we expect default to be "" too
	}

	if c.BurnAmount == 0 {
		c.BurnAmount = defaults.BurnAmount // at this moment, we expect default to be 0 too
	}

	if c.TransactionTopic == "" {
		c.TransactionTopic = defaults.TransactionTopic
	}

	if c.CommitTopic == "" {
		c.CommitTopic = defaults.CommitTopic
	}

	if len(hc.ValidatorGenerators) > 0 {
		for _, generatorName := range hc.ValidatorGenerators {
			generator, ok := validatorGeneratorRegistry[generatorName]
			if !ok {
				return nil, fmt.Errorf("error no generator of name %s", generatorName)
			}
			c.ValidatorGenerators = append(c.ValidatorGenerators, generator)
		}
	} else {
		c.ValidatorGenerators = defaults.ValidatorGenerators
	}

	if len(hc.Transactions) > 0 {
		c.Transactions = make(map[transactions.Transaction_Type]chaintree.TransactorFunc)
		for _, transactionName := range hc.Transactions {
			enum, ok := transactions.Transaction_Type_value[transactionName]
			if !ok {
				return nil, fmt.Errorf("error: you must specify a name that is specified in transactions protobufs")
			}
			fn, ok := transactorRegistry[transactionName]
			if !ok {
				return nil, fmt.Errorf("error: you must specify a name that is registered using RegisterTransactor")
			}
			c.Transactions[transactions.Transaction_Type(enum)] = fn
		}
	} else {
		c.Transactions = defaults.Transactions
	}

	signers := make([]PublicKeySet, len(hc.Signers))
	for i, humanPub := range hc.Signers {
		pub, err := bytesToKeys(humanPub)
		if err != nil {
			return nil, fmt.Errorf("error getting signer from human: %v", err)
		}
		signers[i] = pub
	}
	c.Signers = signers

	return c, nil
}

// TomlToConfig will load a notary group config from a toml string
// Generally these configs are nested and so you will rarely need
// to use this function, it's more to validate example files
// and to use in tests.
func TomlToConfig(tomlBytes string) (*Config, error) {
	var tc TomlConfig
	_, err := toml.Decode(tomlBytes, &tc)
	if err != nil {
		return nil, fmt.Errorf("error decoding toml: %v", err)
	}
	hc, err := tc.toPBConfig()
	if err != nil {
		return nil, fmt.Errorf("error converting toml config to config")
	}
	return HumanConfigToConfig(hc)
}
