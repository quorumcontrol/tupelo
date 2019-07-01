package wallet

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"log"

	datastore "github.com/ipfs/go-datastore"
	query "github.com/ipfs/go-datastore/query"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	cid "github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/tupelo-go-sdk/consensus"
	"github.com/quorumcontrol/tupelo/wallet/adapters"
)

// just make sure that implementation conforms to the interface
var _ consensus.Wallet = (*Wallet)(nil)

// Wallet stores keys and metadata about a chaintree (id, signatures, storage adapter / config)
type Wallet struct {
	storage  datastore.Datastore
	adapters *adapters.AdapterSingletonFactory
}

type WalletConfig struct {
	Storage datastore.Datastore
}

type ExistingChainError struct {
	publicKey *ecdsa.PublicKey
}

func (e ExistingChainError) Error() string {
	keyAddr := crypto.PubkeyToAddress(*e.publicKey).String()
	return fmt.Sprintf("A chain tree for public key %v has already been created.", keyAddr)
}

func NewWallet(config *WalletConfig) *Wallet {
	return &Wallet{
		storage:  config.Storage,
		adapters: adapters.NewAdapterSingletonFactory(),
	}
}

func (w *Wallet) Storage() datastore.Datastore {
	return w.storage
}

func (w *Wallet) Close() {
	w.storage.Close()
	w.adapters.Close()
}

func (w *Wallet) GetTip(chainId string) ([]byte, error) {
	tip, err := w.storage.Get(chainStorageKey(chainId))
	if err != nil {
		return nil, fmt.Errorf("error getting tip for chain id %v: %v", chainId, err)
	}

	return tip, nil
}

func (w *Wallet) GetChain(chainId string) (*consensus.SignedChainTree, error) {
	ctx := context.TODO()

	tip, err := w.GetTip(chainId)
	if err != nil {
		return nil, fmt.Errorf("error getting chain: %v", err)
	}

	signatures, err := w.storage.Get(signatureStorageKey(chainId))
	if err != nil {
		return nil, fmt.Errorf("error getting signatures: %v", err)
	}

	sigs := make(consensus.SignatureMap)
	if len(signatures) > 0 {
		err = cbornode.DecodeInto(signatures, &sigs)
		if err != nil {
			return nil, fmt.Errorf("error decoding signatures: %v", err)
		}
	}

	tipCid, err := cid.Cast(tip)
	if err != nil {
		return nil, fmt.Errorf("error casting tip: %v", err)
	}

	adapter, err := w.storageAdapterForChain(chainId)
	if err != nil {
		return nil, fmt.Errorf("error fetching adapter: %v", err)
	}
	storedTree := dag.NewDag(ctx, tipCid, adapter.Store())

	tree, err := chaintree.NewChainTree(ctx, storedTree, nil, consensus.DefaultTransactors)
	if err != nil {
		return nil, fmt.Errorf("error creating tree: %v", err)
	}

	return &consensus.SignedChainTree{
		ChainTree:  tree,
		Signatures: sigs,
	}, nil
}

func (w *Wallet) CreateChain(keyAddr string, storageConfig *adapters.Config) (*consensus.SignedChainTree, error) {
	key, err := w.GetKey(keyAddr)
	if err != nil {
		return nil, fmt.Errorf("Error getting key: %v", err)
	}

	if w.ChainExists(keyAddr) {
		return nil, ExistingChainError{publicKey: &key.PublicKey}
	}

	chain, err := consensus.NewSignedChainTree(key.PublicKey, nodestore.MustMemoryStore(context.TODO()))
	if err != nil {
		return nil, err
	}

	chainId, err := chain.Id()
	if err != nil {
		return nil, err
	}

	err = w.ConfigureChainStorage(chainId, storageConfig)
	if err != nil {
		return nil, err
	}

	err = w.SaveChain(chain)
	if err != nil {
		return nil, err
	}

	return chain, err
}

func (w *Wallet) ConfigureChainStorage(chainId string, storageConfig *adapters.Config) error {
	storageConfigBytes, err := json.Marshal(storageConfig)
	if err != nil {
		return err
	}
	return w.storage.Put(datastoreConfigStorageKey(chainId), storageConfigBytes)
}

func (w *Wallet) SaveChain(signedChain *consensus.SignedChainTree) error {
	ctx := context.TODO()

	chainId, err := signedChain.Id()
	if err != nil {
		return fmt.Errorf("error getting signedChain id: %v", err)
	}

	adapter, err := w.storageAdapterForChain(chainId)
	if err != nil {
		return fmt.Errorf("error fetching adapter: %v", err)
	}

	nodes, err := signedChain.ChainTree.Dag.Nodes(ctx)
	if err != nil {
		log.Printf("error getting nodes: %s", err)
		// TODO: Enable
		// return fmt.Errorf("error getting nodes: %s", err)
	}
	for _, node := range nodes {
		if err = adapter.Store().Add(ctx, node); err != nil {
			log.Printf("failed to store cbor node: %s", err)
			// TODO: Enable
			// return fmt.Errorf("failed to store cbor node: %s", err)
		}
	}

	return w.SaveChainMetadata(signedChain)
}

func (w *Wallet) SaveChainMetadata(signedChain *consensus.SignedChainTree) error {
	chainId, err := signedChain.Id()
	if err != nil {
		return fmt.Errorf("error getting signedChain id: %v", err)
	}

	sw := &safewrap.SafeWrap{}
	signatureNode := sw.WrapObject(signedChain.Signatures)
	if sw.Err != nil {
		return fmt.Errorf("error wrapping signatures: %v", sw.Err)
	}

	if err = w.storage.Put(signatureStorageKey(chainId), signatureNode.RawData()); err != nil {
		log.Printf("failed to store node data")
		// TODO: Enable
		// return fmt.Errorf("failed to store node data: %s", err)
	}
	if err = w.storage.Put(chainStorageKey(chainId), signedChain.ChainTree.Dag.Tip.Bytes()); err != nil {
		log.Printf("failed to store chain tree tip")
		// TODO: Enable
		// return fmt.Errorf("failed to store chain tree tip: %s", err)
	}

	return nil
}

func (w *Wallet) ChainExistsForKey(keyAddr string) bool {
	key, err := w.GetKey(keyAddr)
	if err != nil {
		return false
	}
	chainId := consensus.EcdsaPubkeyToDid(key.PublicKey)
	return w.ChainExists(chainId)
}

func (w *Wallet) ChainExists(chainId string) bool {
	tip, _ := w.GetTip(chainId)
	return len(tip) > 0
}

func (w *Wallet) GetChainIds() ([]string, error) {
	result, err := w.storage.Query(query.Query{
		Prefix: datastore.NewKey(chainPrefix).String(),
	})

	if err != nil {
		return nil, fmt.Errorf("error getting saved chain tree ids; %v", err)
	}
	defer result.Close()

	var stringIds []string
	for entry := range result.Next() {
		stringIds = append(stringIds, datastore.NewKey(entry.Key).BaseNamespace())
	}

	return stringIds, nil
}

func (w *Wallet) GetKey(addr string) (*ecdsa.PrivateKey, error) {
	keyBytes, err := w.storage.Get(keyStorageKey(common.HexToAddress(addr).String()))
	if err != nil {
		return nil, fmt.Errorf("error getting key: %v", err)
	}
	return crypto.ToECDSA(keyBytes)
}

func (w *Wallet) GenerateKey() (*ecdsa.PrivateKey, error) {
	key, err := crypto.GenerateKey()

	if err != nil {
		return nil, fmt.Errorf("error generating key: %v", err)
	}

	err = w.storage.Put(keyStorageKey(crypto.PubkeyToAddress(key.PublicKey).String()), crypto.FromECDSA(key))
	if err != nil {
		return nil, fmt.Errorf("error generating key: %v", err)
	}

	return key, nil
}

func (w *Wallet) ListKeys() ([]string, error) {
	result, err := w.storage.Query(query.Query{
		Prefix: datastore.NewKey(keyPrefix).String(),
	})
	if err != nil {
		return nil, fmt.Errorf("error getting saved chain tree ids; %v", err)
	}
	defer result.Close()

	var addrs []string

	for entry := range result.Next() {
		addrs = append(addrs, datastore.NewKey(entry.Key).BaseNamespace())
	}

	return addrs, nil
}

func (w *Wallet) storageAdapterForChain(chainId string) (adapters.Adapter, error) {
	configB, err := w.storage.Get(datastoreConfigStorageKey(chainId))

	if err != nil {
		return nil, fmt.Errorf("Could not fetch storage adapter %v", err)
	}

	if len(configB) == 0 {
		return nil, fmt.Errorf("No storage configured for chaintree %v", chainId)
	}

	var config adapters.Config
	err = json.Unmarshal(configB, &config)
	if err != nil {
		return nil, fmt.Errorf("Could not parse storage adapter %v", err)
	}
	return w.adapters.New(&config)
}

const (
	chainPrefix     = "-c-"
	keyPrefix       = "-k-"
	signaturePrefix = "-s-"
	datastorePrefix = "-d-"
)

func chainStorageKey(chainID string) datastore.Key {
	return datastore.KeyWithNamespaces([]string{chainPrefix, chainID})
}

func signatureStorageKey(chainID string) datastore.Key {
	return datastore.KeyWithNamespaces([]string{signaturePrefix, chainID})
}

func datastoreConfigStorageKey(chainID string) datastore.Key {
	return datastore.KeyWithNamespaces([]string{datastorePrefix, chainID})
}

func keyStorageKey(addr string) datastore.Key {
	return datastore.KeyWithNamespaces([]string{keyPrefix, addr})
}
