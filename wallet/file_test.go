package wallet

import (
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/qc3/signer"
	"github.com/stretchr/testify/assert"
	"os"
	"sort"
	"testing"
)

func newSavedChain(t *testing.T, fw *FileWallet, id string) *chaintree.ChainTree {
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
		signer.Transactors,
	)
	assert.Nil(t, err)

	err = fw.SaveChain(chainTree)
	assert.Nil(t, err)

	return chainTree
}

func TestFileWallet_GetChain(t *testing.T) {
	os.RemoveAll("testtmp")
	os.MkdirAll("testtmp", 0700)
	defer os.RemoveAll("testtmp")

	fw := NewFileWallet("password", "testtmp/filewallet")
	defer fw.Close()

	chainTree := newSavedChain(t, fw, "test")

	savedTree, err := fw.GetChain("test")
	assert.Nil(t, err)

	assert.Equal(t, len(chainTree.Dag.Nodes()), len(savedTree.Dag.Nodes()))

	origCids := make([]string, len(chainTree.Dag.Nodes()))
	newCids := make([]string, len(savedTree.Dag.Nodes()))

	for i, node := range chainTree.Dag.Nodes() {
		origCids[i] = node.Node.Cid().String()
	}

	for i, node := range savedTree.Dag.Nodes() {
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
