package wallet

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	cid "github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWallet_GetChain(t *testing.T) {
	w := NewWallet(&WalletConfig{Storage: storage.NewMemStorage()})
	defer w.Close()

	os.RemoveAll("testtmp")
	os.MkdirAll("testtmp", 0700)
	defer os.RemoveAll("testtmp")

	key, err := w.GenerateKey()
	require.Nil(t, err)

	keyAddr := crypto.PubkeyToAddress(key.PublicKey).String()

	newChain, err := w.CreateChain(keyAddr, &StorageAdapterConfig{
		Adapter: "badger",
		Arguments: map[string]interface{}{
			"path": "testtmp/adapter",
		},
	})
	require.Nil(t, err)

	err = w.SaveChain(newChain)
	require.Nil(t, err)

	savedChain, err := w.GetChain(newChain.MustId())
	assert.Nil(t, err)

	assert.Equal(t, newChain.Tip(), savedChain.Tip())

	origNodes, err := newChain.ChainTree.Dag.Nodes()
	require.Nil(t, err)

	savedNodes, err := savedChain.ChainTree.Dag.Nodes()
	require.Nil(t, err)
	assert.Equal(t, len(origNodes), len(savedNodes))

	origCids := make([]string, len(origNodes))
	newCids := make([]string, len(savedNodes))

	for i, node := range origNodes {
		origCids[i] = node.Cid().String()
	}

	for i, node := range savedNodes {
		newCids[i] = node.Cid().String()
	}
	sort.Strings(origCids)
	sort.Strings(newCids)

	assert.Equal(t, origCids, newCids)
}

func TestWallet_SaveChain(t *testing.T) {
	w := NewWallet(&WalletConfig{Storage: storage.NewMemStorage()})
	defer w.Close()

	os.RemoveAll("testtmp")
	os.MkdirAll("testtmp", 0700)
	defer os.RemoveAll("testtmp")

	key, err := w.GenerateKey()
	require.Nil(t, err)

	keyAddr := crypto.PubkeyToAddress(key.PublicKey).String()

	signedTree, err := w.CreateChain(keyAddr, &StorageAdapterConfig{
		Adapter: "badger",
		Arguments: map[string]interface{}{
			"path": "testtmp/adapter",
		},
	})
	require.Nil(t, err)

	err = w.SaveChain(signedTree)
	require.Nil(t, err)

	savedTree, err := w.GetChain(signedTree.MustId())
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

	err = w.SaveChain(signedTree)
	require.Nil(t, err)

	savedTree, err = w.GetChain(signedTree.MustId())
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

	newTree := signedTree.ChainTree.Dag.WithNewTip(signedTree.ChainTree.Dag.Tip)

	unmarshaledRoot, err := newTree.Get(newTree.Tip)
	require.Nil(t, err)
	require.NotNil(t, unmarshaledRoot)

	root := &chaintree.RootNode{}

	err = cbornode.DecodeInto(unmarshaledRoot.RawData(), root)
	require.Nil(t, err)

	require.NotNil(t, root.Tree)

	newTree.Tip = *root.Tree

	newTree.Set(strings.Split("something", "/"), "hi")
	signedTree.ChainTree.Dag.SetAsLink([]string{chaintree.TreeLabel}, newTree)

	chainNode, err := signedTree.ChainTree.Dag.Get(*root.Chain)
	require.Nil(t, err)
	chainData, err := nodestore.CborNodeToObj(chainNode)
	chainMap := chainData.(map[string]interface{})
	require.Nil(t, err)

	sw := &safewrap.SafeWrap{}

	wrappedBlock := sw.WrapObject(blockWithHeaders)
	require.Nil(t, sw.Err)

	lastEntry := &chaintree.ChainEntry{
		PreviousTip:       "",
		BlocksWithHeaders: []cid.Cid{wrappedBlock.Cid()},
	}
	entryNode := sw.WrapObject(lastEntry)
	chainMap["end"] = entryNode.Cid()
	newChainNode := sw.WrapObject(chainMap)

	signedTree.ChainTree.Dag.AddNodes(entryNode)
	signedTree.ChainTree.Dag.AddNodes(wrappedBlock)
	signedTree.ChainTree.Dag.Update([]string{chaintree.ChainLabel}, newChainNode)

	t.Log(signedTree.ChainTree.Dag.Dump())

	err = w.SaveChain(signedTree)
	require.Nil(t, err)

	savedTree, err = w.GetChain(signedTree.MustId())
	require.Nil(t, err)
	savedNodes, err = savedTree.ChainTree.Dag.Nodes()
	require.Nil(t, err)

	assert.Equal(t, len(signedNodes), len(savedNodes))

}

func TestWallet_GetChainIds(t *testing.T) {
	w := NewWallet(&WalletConfig{Storage: storage.NewMemStorage()})
	defer w.Close()

	os.RemoveAll("testtmp")
	os.MkdirAll("testtmp", 0700)
	defer os.RemoveAll("testtmp")

	createdChains := make([]string, 3, 3)

	for i := 0; i < 3; i++ {
		key, err := w.GenerateKey()
		require.Nil(t, err)

		keyAddr := crypto.PubkeyToAddress(key.PublicKey).String()
		signedTree, err := w.CreateChain(keyAddr, &StorageAdapterConfig{
			Adapter: "badger",
			Arguments: map[string]interface{}{
				"path": fmt.Sprintf("testtmp/db-%d", i),
			},
		})
		createdChains[i] = signedTree.MustId()
		require.Nil(t, err)
	}

	ids, err := w.GetChainIds()
	assert.Nil(t, err)

	assert.ElementsMatch(t, createdChains, ids)
}

func TestWallet_GetKey(t *testing.T) {
	w := NewWallet(&WalletConfig{Storage: storage.NewMemStorage()})
	defer w.Close()

	key, err := w.GenerateKey()
	assert.Nil(t, err)

	retKey, err := w.GetKey(crypto.PubkeyToAddress(key.PublicKey).String())
	assert.Equal(t, retKey, key)
}

func TestWallet_ChainExists(t *testing.T) {
	w := NewWallet(&WalletConfig{Storage: storage.NewMemStorage()})
	defer w.Close()

	os.RemoveAll("testtmp")
	os.MkdirAll("testtmp", 0700)
	defer os.RemoveAll("testtmp")

	key, err := w.GenerateKey()
	require.Nil(t, err)

	keyAddr := crypto.PubkeyToAddress(key.PublicKey).String()
	chainId := consensus.EcdsaPubkeyToDid(key.PublicKey)

	assert.False(t, w.ChainExistsForKey(keyAddr))
	assert.False(t, w.ChainExists(chainId))

	signedTree, err := w.CreateChain(keyAddr, &StorageAdapterConfig{
		Adapter: "badger",
		Arguments: map[string]interface{}{
			"path": "testtmp/adapter",
		},
	})
	require.Nil(t, err)

	assert.True(t, w.ChainExistsForKey(keyAddr))
	assert.True(t, w.ChainExists(signedTree.MustId()))
}
