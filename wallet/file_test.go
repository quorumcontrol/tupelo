package wallet

import (
	"os"
	"sort"
	"testing"

	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/storage"

	"crypto/ecdsa"

	"strings"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSavedChain(t *testing.T, fw *FileWallet, key ecdsa.PublicKey) *consensus.SignedChainTree {
	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())

	signedTree, err := consensus.NewSignedChainTree(key, nodeStore)
	require.Nil(t, err)

	err = fw.SaveChain(signedTree)
	assert.Nil(t, err)

	return signedTree
}

func TestFileWallet_Create(t *testing.T) {
	os.RemoveAll("testtmp")
	os.MkdirAll("testtmp", 0700)
	defer os.RemoveAll("testtmp")

	fw := NewFileWallet("testtmp/filewallet")
	err := fw.Create("password")
	require.Nil(t, err, "Create should succeed on the first try.")

	fw2 := NewFileWallet("testtmp/filewallet")
	err2 := fw2.Create("password")
	require.Error(t, err2, "Create should error on the second try.")
}

func TestFileWallet_Unlock(t *testing.T) {
	os.RemoveAll("testtmp")
	os.MkdirAll("testtmp", 0700)
	defer os.RemoveAll("testtmp")

	fw := NewFileWallet("testtmp/filewallet")
	err := fw.Unlock("password")
	require.Error(t, err, "Unlock should fail without a previous Create.")

	err = fw.Create("password")
	require.Nil(t, err, "Create should succeed on the first try.")
	fw.Close()

	fw2 := NewFileWallet("testtmp/filewallet")
	err2 := fw2.Unlock("password")
	require.Nil(t, err2, "Unlock should succeed after a previous Create.")
}

func TestFileWallet_GetChain(t *testing.T) {
	os.RemoveAll("testtmp")
	os.MkdirAll("testtmp", 0700)
	defer os.RemoveAll("testtmp")

	fw := NewFileWallet("testtmp/filewallet")
	fw.CreateIfNotExists("password")
	defer fw.Close()

	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	signedTree := newSavedChain(t, fw, key.PublicKey)

	savedTree, err := fw.GetChain(signedTree.MustId())
	assert.Nil(t, err)
	signedNodes, err := signedTree.ChainTree.Dag.Nodes()
	require.Nil(t, err)

	savedNodes, err := savedTree.ChainTree.Dag.Nodes()
	require.Nil(t, err)
	assert.Equal(t, len(signedNodes), len(savedNodes))

	origCids := make([]string, len(signedNodes))
	newCids := make([]string, len(savedNodes))

	for i, node := range signedNodes {
		origCids[i] = node.Cid().String()
	}

	for i, node := range savedNodes {
		newCids[i] = node.Cid().String()
	}
	sort.Strings(origCids)
	sort.Strings(newCids)

	assert.Equal(t, origCids, newCids)
}

func TestFileWallet_SaveChain(t *testing.T) {
	os.RemoveAll("testtmp")
	os.MkdirAll("testtmp", 0700)
	defer os.RemoveAll("testtmp")

	fw := NewFileWallet("testtmp/filewallet")
	fw.CreateIfNotExists("password")
	defer fw.Close()

	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	signedTree := newSavedChain(t, fw, key.PublicKey)

	savedTree, err := fw.GetChain(signedTree.MustId())
	assert.Nil(t, err)
	signedNodes, err := signedTree.ChainTree.Dag.Nodes()
	require.Nil(t, err)

	savedNodes, err := savedTree.ChainTree.Dag.Nodes()
	require.Nil(t, err)
	assert.Equal(t, len(signedNodes), len(savedNodes))

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

	savedNodes, err = savedTree.ChainTree.Dag.Nodes()
	require.Nil(t, err)

	assert.Equal(t, len(signedNodes), len(savedNodes))

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

	newTree := signedTree.ChainTree.Dag.WithNewTip(signedTree.ChainTree.Dag.Tip)

	unmarshaledRoot, err := newTree.Get(newTree.Tip)
	require.Nil(t, err)
	require.NotNil(t, unmarshaledRoot)

	root := &chaintree.RootNode{}

	err = cbornode.DecodeInto(unmarshaledRoot.RawData(), root)
	require.Nil(t, err)

	require.NotNil(t, root.Tree)

	newTree.Tip = root.Tree

	newTree.Set(strings.Split("something", "/"), "hi")
	signedTree.ChainTree.Dag.SetAsLink([]string{chaintree.TreeLabel}, newTree)

	chainNode, err := signedTree.ChainTree.Dag.Get(root.Chain)
	require.Nil(t, err)
	chainMap, err := nodestore.CborNodeToObj(chainNode)
	require.Nil(t, err)

	sw := &safewrap.SafeWrap{}

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
	signedTree.ChainTree.Dag.Swap(chainNode.Cid(), newChainNode)

	t.Log(signedTree.ChainTree.Dag.Dump())

	err = fw.SaveChain(signedTree)
	require.Nil(t, err)

	savedTree, err = fw.GetChain(signedTree.MustId())
	require.Nil(t, err)
	savedNodes, err = savedTree.ChainTree.Dag.Nodes()
	require.Nil(t, err)

	assert.Equal(t, len(signedNodes), len(savedNodes))

}

func TestFileWallet_GetChainIds(t *testing.T) {
	os.RemoveAll("testtmp")
	os.MkdirAll("testtmp", 0700)
	defer os.RemoveAll("testtmp")

	fw := NewFileWallet("testtmp/filewallet")
	fw.CreateIfNotExists("password")
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

	fw := NewFileWallet("testtmp/filewallet")
	fw.CreateIfNotExists("password")
	defer fw.Close()

	key, err := fw.GenerateKey()
	assert.Nil(t, err)

	retKey, err := fw.GetKey(crypto.PubkeyToAddress(key.PublicKey).String())

	assert.Equal(t, retKey, key)
}
