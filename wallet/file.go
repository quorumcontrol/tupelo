package wallet

import (
	"crypto/ecdsa"
	"fmt"

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

// just make sure that implementation conforms to the interface
var _ consensus.Wallet = (*FileWallet)(nil)

type FileWallet struct {
	boltStorage storage.EncryptedStorage
	nodeStore   nodestore.NodeStore
}

func NewFileWallet(passphrase, path string) *FileWallet {
	boltStorage := storage.NewEncryptedBoltStorage(path)
	boltStorage.Unlock(passphrase)
	boltStorage.CreateBucketIfNotExists(chainBucket)
	boltStorage.CreateBucketIfNotExists(keyBucket)
	boltStorage.CreateBucketIfNotExists(nodeBucket)
	boltStorage.CreateBucketIfNotExists(signaturesBucket)
	return &FileWallet{
		boltStorage: boltStorage,
	}
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
