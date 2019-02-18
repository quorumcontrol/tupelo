package wallet

import (
	"crypto/ecdsa"
	"encoding/json"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	cid "github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/quorumcontrol/tupelo/wallet/adapters"
)

// just make sure that implementation conforms to the interface
var _ consensus.Wallet = (*Wallet)(nil)

// Wallet stores keys and metadata about a chaintree (id, signatures, storage adapter / config)
type Wallet struct {
	storage  storage.Storage
	adapters *adapters.AdapterSingletonFactory
}

type WalletConfig struct {
	Storage storage.Storage
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

func (w *Wallet) Storage() storage.Storage {
	return w.storage
}

func (w *Wallet) Close() {
	w.storage.Close()
	w.adapters.Close()
}

func (w *Wallet) GetTip(chainId string) ([]byte, error) {
	tip, err := w.storage.Get(chainStorageKey([]byte(chainId)))
	if err != nil {
		return nil, fmt.Errorf("error getting tip for chain id %v: %v", chainId, err)
	}

	return tip, nil
}

func (w *Wallet) GetChain(chainId string) (*consensus.SignedChainTree, error) {
	tip, err := w.GetTip(chainId)
	if err != nil {
		return nil, fmt.Errorf("error getting chain: %v", err)
	}

	signatures, err := w.storage.Get(signatureStorageKey([]byte(chainId)))
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
	storedTree := dag.NewDag(tipCid, adapter.Store())

	nodes, err := storedTree.Nodes()
	if err != nil {
		return nil, fmt.Errorf("error fetching stored nodes: %v", err)
	}

	memoryStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	memoryTree := dag.NewDag(tipCid, memoryStore)
	memoryTree.AddNodes(nodes...)

	tree, err := chaintree.NewChainTree(memoryTree, nil, consensus.DefaultTransactors)
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

	chain, err := consensus.NewSignedChainTree(key.PublicKey, nodestore.NewStorageBasedStore(storage.NewMemStorage()))
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
	return w.storage.Set(datastoreConfigStorageKey([]byte(chainId)), storageConfigBytes)
}

func (w *Wallet) SaveChain(signedChain *consensus.SignedChainTree) error {
	chainId, err := signedChain.Id()
	if err != nil {
		return fmt.Errorf("error getting signedChain id: %v", err)
	}

	adapter, err := w.storageAdapterForChain(chainId)
	if err != nil {
		return fmt.Errorf("error fetching adapter: %v", err)
	}

	nodes, err := signedChain.ChainTree.Dag.Nodes()
	if err != nil {
		return fmt.Errorf("error getting nodes: %v", err)
	}
	for _, node := range nodes {
		adapter.Store().StoreNode(node)
	}

	sw := &safewrap.SafeWrap{}
	signatureNode := sw.WrapObject(signedChain.Signatures)
	if sw.Err != nil {
		return fmt.Errorf("error wrapping signatures: %v", sw.Err)
	}

	w.storage.Set(signatureStorageKey([]byte(chainId)), signatureNode.RawData())
	w.storage.Set(chainStorageKey([]byte(chainId)), signedChain.ChainTree.Dag.Tip.Bytes())

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
	return tip != nil && len(tip) > 0
}

func (w *Wallet) GetChainIds() ([]string, error) {
	chainIds, err := w.storage.GetKeysByPrefix(chainPrefix)
	if err != nil {
		return nil, fmt.Errorf("error getting saved chain tree ids; %v", err)
	}

	stringIds := make([]string, len(chainIds))
	for i, k := range chainIds {
		stringIds[i] = string(k[len(chainPrefix):])
	}

	return stringIds, nil
}

func (w *Wallet) GetKey(addr string) (*ecdsa.PrivateKey, error) {
	keyBytes, err := w.storage.Get(keyStorageKey(common.HexToAddress(addr).Bytes()))
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

	err = w.storage.Set(keyStorageKey(crypto.PubkeyToAddress(key.PublicKey).Bytes()), crypto.FromECDSA(key))
	if err != nil {
		return nil, fmt.Errorf("error generating key: %v", err)
	}

	return key, nil
}

func (w *Wallet) ListKeys() ([]string, error) {
	keys, err := w.storage.GetKeysByPrefix(keyPrefix)
	if err != nil {
		return nil, fmt.Errorf("error getting keys; %v", err)
	}
	addrs := make([]string, len(keys))
	for i, k := range keys {
		addrs[i] = common.BytesToAddress(k[len(keyPrefix):]).String()
	}
	return addrs, nil
}

func (w *Wallet) storageAdapterForChain(chainId string) (adapters.Adapter, error) {
	configB, err := w.storage.Get(datastoreConfigStorageKey([]byte(chainId)))

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

var chainPrefix = []byte("-c-")
var keyPrefix = []byte("-k-")
var signaturePrefix = []byte("-s-")
var datastorePrefix = []byte("-d-")

func chainStorageKey(chainID []byte) []byte {
	return append(chainPrefix, chainID...)
}

func signatureStorageKey(chainID []byte) []byte {
	return append(signaturePrefix, chainID...)
}

func datastoreConfigStorageKey(chainID []byte) []byte {
	return append(datastorePrefix, chainID...)
}

func keyStorageKey(addr []byte) []byte {
	return append(keyPrefix, addr...)
}
