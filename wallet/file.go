package wallet

import (
	"fmt"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/qc3/signer"
	"github.com/quorumcontrol/qc3/storage"
)

var chainBucket = []byte("chains")
var keyBucket = []byte("keys")
var nodeBucket = []byte("nodes")

// just make sure that implementation conforms to the interface
//var _ Wallet = (*FileWallet)(nil)

type FileWallet struct {
	boltStorage storage.EncryptedStorage
}

func NewFileWallet(passphrase, path string) *FileWallet {
	boltStorage := storage.NewEncryptedBoltStorage(path)
	boltStorage.Unlock(passphrase)
	boltStorage.CreateBucketIfNotExists(chainBucket)
	boltStorage.CreateBucketIfNotExists(keyBucket)
	boltStorage.CreateBucketIfNotExists(nodeBucket)
	return &FileWallet{
		boltStorage: boltStorage,
	}
}

func (fw *FileWallet) Close() {
	fw.boltStorage.Close()
}

func (fw *FileWallet) getAllNodes(cid []byte) ([]*cbornode.Node, error) {
	nodes := make([]*cbornode.Node, 1)
	nodeBytes, err := fw.boltStorage.Get(nodeBucket, cid)
	if err != nil {
		return nil, fmt.Errorf("error getting nodes: %v", err)
	}

	sw := dag.SafeWrap{}
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

func (fw *FileWallet) GetChain(id string) (*chaintree.ChainTree, error) {
	tip, err := fw.boltStorage.Get(chainBucket, []byte(id))
	if err != nil {
		return nil, fmt.Errorf("error getting chain: %v", err)
	}

	tipCid, err := cid.Cast(tip)
	if err != nil {
		return nil, fmt.Errorf("error casting tip: %v", err)
	}

	nodes, err := fw.getAllNodes(tip)
	if err != nil {
		return nil, fmt.Errorf("error getting nodes: %v", err)
	}

	dag := dag.NewBidirectionalTree(tipCid, nodes...)

	tree, err := chaintree.NewChainTree(dag, nil, signer.Transactors)
	if err != nil {
		return nil, fmt.Errorf("error creating tree: %v", err)
	}

	return tree, nil
}

func (fw *FileWallet) SaveChain(chain *chaintree.ChainTree) error {
	nodes := chain.Dag.Nodes()
	for _, node := range nodes {
		fw.boltStorage.Set(nodeBucket, node.Node.Cid().Bytes(), node.Node.RawData())
	}

	id, err := chain.Id()
	if err != nil {
		return fmt.Errorf("error getting chain id: %v", err)
	}

	fw.boltStorage.Set(chainBucket, []byte(id), chain.Dag.Tip.Bytes())

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
