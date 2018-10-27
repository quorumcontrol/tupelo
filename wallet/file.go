package wallet

import (
	"crypto/ecdsa"
	"fmt"
	"os"

	"github.com/quorumcontrol/chaintree/nodestore"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/storage"
)

var chainBucket = []byte("chains")
var signaturesBucket = []byte("signatures")
var keyBucket = []byte("keys")
var nodeBucket = []byte("nodes")

type UnlockInexistentWalletError struct {
	path string
}

func (e *UnlockInexistentWalletError) Error() string {
	return fmt.Sprintf("Can't unlock wallet at path '%s'. It does not exist. Create it first", e.path)
}

type CreateExistingWalletError struct {
	path string
}

func (e *CreateExistingWalletError) Error() string {
	return fmt.Sprintf("Can't create wallet at path '%s'. Another wallet already exists at the same path.", e.path)
}

// just make sure that implementation conforms to the interface
var _ consensus.Wallet = (*FileWallet)(nil)

type FileWallet struct {
	path        string
	boltStorage storage.EncryptedStorage
	nodeStore   nodestore.NodeStore
}

// isExists checks if the wallet specified by `path` already exists.
func (fw *FileWallet) isExists() bool {
	_, err := os.Stat(fw.path)

	return !os.IsNotExist(err)
}

func NewFileWallet(path string) *FileWallet {
	return &FileWallet{
		path: path,
	}
}

// CreateIfNotExists creates a new wallet at the path specified by `fw` if one
// hasn't already been created at that path.
func (fw *FileWallet) CreateIfNotExists(passphrase string) {
	boltStorage := storage.NewEncryptedBoltStorage(fw.path)
	boltStorage.Unlock(passphrase)
	boltStorage.CreateBucketIfNotExists(chainBucket)
	boltStorage.CreateBucketIfNotExists(keyBucket)
	boltStorage.CreateBucketIfNotExists(nodeBucket)
	boltStorage.CreateBucketIfNotExists(signaturesBucket)

	fw.boltStorage = boltStorage
}

// Create creates a new wallet at the path specified by `fw`. It returns an
// error if a wallet already exists at that path, and nil otherwise.
func (fw *FileWallet) Create(passphrase string) error {
	if fw.isExists() {
		return &CreateExistingWalletError{
			path: fw.path,
		}
	}

	fw.CreateIfNotExists(passphrase)

	return nil
}

// Unlock opens a pre-existing wallet after first validating `passphrase`
// against that wallet. It returns an error if the passphrase is not correct or
// the wallet doesn't already exist, and nil otherwise.
func (fw *FileWallet) Unlock(passphrase string) error {
	if !fw.isExists() {
		return &UnlockInexistentWalletError{
			path: fw.path,
		}
	}

	boltStorage := storage.NewEncryptedBoltStorage(fw.path)
	boltStorage.Unlock(passphrase)

	fw.boltStorage = boltStorage

	return nil
}

func (fw *FileWallet) Close() {
	fw.boltStorage.Close()
}

// NodeStore returns a NodeStore based on the underlying storage system of the wallet
func (fw *FileWallet) NodeStore() nodestore.NodeStore {
	if fw.nodeStore == nil { //TODO: thread safety
		fw.nodeStore = nodestore.NewStorageBasedStore(fw.boltStorage)
	}
	return fw.nodeStore
}

func (fw *FileWallet) getAllNodes(objCid []byte) ([]*cbornode.Node, error) {
	nodes := make([]*cbornode.Node, 1)
	nodeBytes, err := fw.boltStorage.Get(nodeBucket, objCid)
	if err != nil {
		return nil, fmt.Errorf("error getting nodes: %v", err)
	}
	if len(nodeBytes) == 0 {
		id, _ := cid.Cast(objCid)
		return nil, fmt.Errorf("error, node not stored: %s ", id.String())
	}

	sw := safewrap.SafeWrap{}
	node := sw.Decode(nodeBytes)
	if sw.Err != nil {
		return nil, fmt.Errorf("error decoding: %v", err)
	}
	nodes[0] = node

	links := node.Links()
	for _, link := range links {
		linkNodes, err := fw.getAllNodes(link.Cid.Bytes())
		if err != nil {
			return nil, fmt.Errorf("error getting links: %v", err)
		}
		nodes = append(nodes, linkNodes...)
	}

	return nodes, nil
}

func (fw *FileWallet) GetChain(id string) (*consensus.SignedChainTree, error) {
	tip, err := fw.boltStorage.Get(chainBucket, []byte(id))
	if err != nil {
		return nil, fmt.Errorf("error getting chain: %v", err)
	}

	signatures, err := fw.boltStorage.Get(signaturesBucket, []byte(id+"_signatures"))
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

	nodes, err := fw.getAllNodes(tip)
	if err != nil {
		return nil, fmt.Errorf("error getting nodes: %v", err)
	}

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())

	dagTree := dag.NewDag(tipCid, nodeStore)
	dagTree.AddNodes(nodes...)

	tree, err := chaintree.NewChainTree(dagTree, nil, consensus.DefaultTransactors)
	if err != nil {
		return nil, fmt.Errorf("error creating tree: %v", err)
	}

	return &consensus.SignedChainTree{
		ChainTree:  tree,
		Signatures: sigs,
	}, nil
}

func (fw *FileWallet) SaveChain(signedChain *consensus.SignedChainTree) error {
	nodes, err := signedChain.ChainTree.Dag.Nodes()
	if err != nil {
		return fmt.Errorf("error getting nodes: %v", err)
	}
	for _, node := range nodes {
		fw.boltStorage.Set(nodeBucket, node.Cid().Bytes(), node.RawData())
	}

	sw := &safewrap.SafeWrap{}
	signatureNode := sw.WrapObject(signedChain.Signatures)
	if sw.Err != nil {
		return fmt.Errorf("error wrapping signatures: %v", sw.Err)
	}

	id, err := signedChain.Id()
	if err != nil {
		return fmt.Errorf("error getting signedChain id: %v", err)
	}

	fw.boltStorage.Set(signaturesBucket, []byte(id+"_signatures"), signatureNode.RawData())
	fw.boltStorage.Set(chainBucket, []byte(id), signedChain.ChainTree.Dag.Tip.Bytes())

	return nil
}

func (fw *FileWallet) GetChainIds() ([]string, error) {
	keys, err := fw.boltStorage.GetKeys(chainBucket)
	if err != nil {
		return nil, fmt.Errorf("error getting keys; %v", err)
	}
	ids := make([]string, len(keys))
	for i, k := range keys {
		ids[i] = string(k)
	}
	return ids, nil
}

func (fw *FileWallet) GetKey(addr string) (*ecdsa.PrivateKey, error) {
	keyBytes, err := fw.boltStorage.Get(keyBucket, common.HexToAddress(addr).Bytes())
	if err != nil {
		return nil, fmt.Errorf("error getting key: %v", err)
	}
	return crypto.ToECDSA(keyBytes)
}

func (fw *FileWallet) GenerateKey() (*ecdsa.PrivateKey, error) {
	key, err := crypto.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("error generating key: %v", err)
	}

	err = fw.boltStorage.Set(keyBucket, crypto.PubkeyToAddress(key.PublicKey).Bytes(), crypto.FromECDSA(key))
	if err != nil {
		return nil, fmt.Errorf("error generating key: %v", err)
	}

	return key, nil
}

func (fw *FileWallet) ListKeys() ([]string, error) {
	keys, err := fw.boltStorage.GetKeys(keyBucket)
	if err != nil {
		return nil, fmt.Errorf("error getting keys; %v", err)
	}
	addrs := make([]string, len(keys))
	for i, k := range keys {
		addrs[i] = common.BytesToAddress(k).String()
	}
	return addrs, nil
}
