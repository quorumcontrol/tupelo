package wallet

import (
	"os"
	"sort"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/stretchr/testify/assert"
)

func newSavedChain(t *testing.T, fw *FileWallet, id string) *consensus.SignedChainTree {
	sw := &dag.SafeWrap{}

	tree := sw.WrapObject(map[string]string{
		"hithere": "hothere",
	})

	chain := sw.WrapObject(make(map[string]string))

	root := sw.WrapObject(map[string]interface{}{
		"chain": chain.Cid(),
		"tree":  tree.Cid(),
		"id":    id,
	})

	chainTree, err := chaintree.NewChainTree(
		dag.NewBidirectionalTree(root.Cid(), root, tree, chain),
		nil,
		consensus.DefaultTransactors,
	)
	assert.Nil(t, err)

	signedTree := &consensus.SignedChainTree{
		ChainTree: chainTree,
	}

	err = fw.SaveChain(signedTree)
	assert.Nil(t, err)

	return signedTree
}

func TestFileWallet_GetChain(t *testing.T) {
	os.RemoveAll("testtmp")
	os.MkdirAll("testtmp", 0700)
	defer os.RemoveAll("testtmp")

	fw := NewFileWallet("password", "testtmp/filewallet")
	defer fw.Close()

	signedTree := newSavedChain(t, fw, "test")

	savedTree, err := fw.GetChain("test")
	assert.Nil(t, err)

	assert.Equal(t, len(signedTree.ChainTree.Dag.Nodes()), len(savedTree.ChainTree.Dag.Nodes()))

	origCids := make([]string, len(signedTree.ChainTree.Dag.Nodes()))
	newCids := make([]string, len(savedTree.ChainTree.Dag.Nodes()))

	for i, node := range signedTree.ChainTree.Dag.Nodes() {
		origCids[i] = node.Node.Cid().String()
	}

	for i, node := range savedTree.ChainTree.Dag.Nodes() {
		newCids[i] = node.Node.Cid().String()
	}
	sort.Strings(origCids)
	sort.Strings(newCids)

	assert.Equal(t, origCids, newCids)
}

func TestFileWallet_GetChainIds(t *testing.T) {
	os.RemoveAll("testtmp")
	os.MkdirAll("testtmp", 0700)
	defer os.RemoveAll("testtmp")

	fw := NewFileWallet("password", "testtmp/filewallet")
	defer fw.Close()

	newSavedChain(t, fw, "test")

	ids, err := fw.GetChainIds()
	assert.Nil(t, err)

	assert.Equal(t, []string{"test"}, ids)
}

func TestFileWallet_GetKey(t *testing.T) {

	os.RemoveAll("testtmp")
	os.MkdirAll("testtmp", 0700)
	defer os.RemoveAll("testtmp")

	fw := NewFileWallet("password", "testtmp/filewallet")
	defer fw.Close()

	key, err := fw.GenerateKey()
	assert.Nil(t, err)

	retKey, err := fw.GetKey(crypto.PubkeyToAddress(key.PublicKey).String())

	assert.Equal(t, retKey, key)
}
