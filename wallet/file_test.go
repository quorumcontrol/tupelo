package wallet

import (
	"os"
	"sort"
	"testing"

	"crypto/ecdsa"

	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSavedChain(t *testing.T, fw *FileWallet, key ecdsa.PublicKey) *consensus.SignedChainTree {
	signedTree, err := consensus.NewSignedChainTree(key)
	require.Nil(t, err)

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

	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	signedTree := newSavedChain(t, fw, key.PublicKey)

	savedTree, err := fw.GetChain(signedTree.MustId())
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

func TestFileWallet_SaveChain(t *testing.T) {
	os.RemoveAll("testtmp")
	os.MkdirAll("testtmp", 0700)
	defer os.RemoveAll("testtmp")

	fw := NewFileWallet("password", "testtmp/filewallet")
	defer fw.Close()

	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	signedTree := newSavedChain(t, fw, key.PublicKey)

	savedTree, err := fw.GetChain(signedTree.MustId())
	assert.Nil(t, err)

	assert.Equal(t, len(signedTree.ChainTree.Dag.Nodes()), len(savedTree.ChainTree.Dag.Nodes()))

	hsh := crypto.Keccak256([]byte("hi"))

	ecdsaSig, _ := crypto.Sign(hsh, key)

	sig := &consensus.Signature{
		Type:      consensus.KeyTypeSecp256k1,
		Signature: ecdsaSig,
	}

	signedTree.Signatures = consensus.SignatureMap{
		"id": *sig,
	}

	err = fw.SaveChain(signedTree)
	require.Nil(t, err)

	savedTree, err = fw.GetChain(signedTree.MustId())
	assert.Nil(t, err)

	assert.Equal(t, len(signedTree.ChainTree.Dag.Nodes()), len(savedTree.ChainTree.Dag.Nodes()))

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				{
					Type: consensus.TransactionTypeSetData,
					Payload: &consensus.SetDataPayload{
						Path:  "something",
						Value: "hi",
					},
				},
			},
		},
	}

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, key)
	require.Nil(t, err)

	//isValid, err := signedTree.ChainTree.ProcessBlock(blockWithHeaders)
	//require.Nil(t, err)
	//require.True(t, isValid)

	newTree := signedTree.ChainTree.Dag.Copy()

	unmarshaledRoot := newTree.Get(newTree.Tip)
	require.NotNil(t, unmarshaledRoot)

	root := &chaintree.RootNode{}

	err = cbornode.DecodeInto(unmarshaledRoot.Node.RawData(), root)
	require.Nil(t, err)

	require.NotNil(t, root.Tree)

	newTree.Tip = root.Tree

	newTree.Set(strings.Split("something", "/"), "hi")
	signedTree.ChainTree.Dag.SetAsLink([]string{chaintree.TreeLabel}, newTree)

	chainNode := signedTree.ChainTree.Dag.Get(root.Chain)
	chainMap, err := chainNode.AsMap()
	require.Nil(t, err)

	sw := &dag.SafeWrap{}

	wrappedBlock := sw.WrapObject(blockWithHeaders)
	require.Nil(t, sw.Err)

	lastEntry := &chaintree.ChainEntry{
		PreviousTip:       "",
		BlocksWithHeaders: []*cid.Cid{wrappedBlock.Cid()},
	}
	entryNode := sw.WrapObject(lastEntry)
	chainMap["end"] = entryNode.Cid()
	newChainNode := sw.WrapObject(chainMap)

	signedTree.ChainTree.Dag.AddNodes(entryNode)
	signedTree.ChainTree.Dag.AddNodes(wrappedBlock)
	signedTree.ChainTree.Dag.Swap(chainNode.Node.Cid(), newChainNode)

	signedTree.ChainTree.Dag.Prune()

	t.Log(signedTree.ChainTree.Dag.Dump())

	err = fw.SaveChain(signedTree)
	require.Nil(t, err)

	savedTree, err = fw.GetChain(signedTree.MustId())
	require.Nil(t, err)

	assert.Equal(t, len(signedTree.ChainTree.Dag.Nodes()), len(savedTree.ChainTree.Dag.Nodes()))

}

func TestFileWallet_GetChainIds(t *testing.T) {
	os.RemoveAll("testtmp")
	os.MkdirAll("testtmp", 0700)
	defer os.RemoveAll("testtmp")

	fw := NewFileWallet("password", "testtmp/filewallet")
	defer fw.Close()

	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	chain := newSavedChain(t, fw, key.PublicKey)

	ids, err := fw.GetChainIds()
	assert.Nil(t, err)

	assert.Equal(t, []string{chain.MustId()}, ids)
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
