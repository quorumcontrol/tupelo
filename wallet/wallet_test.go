package wallet

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/quorumcontrol/messages/build/go/signatures"

	"github.com/ethereum/go-ethereum/crypto"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/build/go/transactions"
	"github.com/quorumcontrol/tupelo/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/quorumcontrol/tupelo-go-sdk/consensus"
	"github.com/quorumcontrol/tupelo/wallet/adapters"
)

func TestMockAdapterWallet(t *testing.T) {
	storageConfig := &adapters.Config{Adapter: "mock"}
	SubtestAll(t, storageConfig)
}

func TestBadgerWallet(t *testing.T) {
	err := os.RemoveAll("testtmp")
	require.Nil(t, err)
	err = os.MkdirAll("testtmp", 0700)
	require.Nil(t, err)
	defer os.RemoveAll("testtmp")

	storageConfig := &adapters.Config{
		Adapter: "badger",
		Arguments: map[string]interface{}{
			"path": "testtmp/adapter",
		},
	}

	SubtestAll(t, storageConfig)
}

func SubtestAll(t *testing.T, storageConfig *adapters.Config) {
	SubtestWallet_GetChain(t, storageConfig)
	SubtestWallet_SaveChain(t, storageConfig)
	SubtestWallet_GetChainIds(t, storageConfig)
	SubtestWallet_ChainExists(t, storageConfig)
	SubtestWallet_GetKey(t, storageConfig)
	SubtestWallet_ListKeys(t, storageConfig)
	SubtestWallet_ChainExists(t, storageConfig)
}

func SubtestWallet_GetChain(t *testing.T, storageConfig *adapters.Config) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := NewWallet(&WalletConfig{Storage: storage.NewDefaultMemory()})
	defer w.Close()

	key, err := w.GenerateKey()
	require.Nil(t, err)

	keyAddr := crypto.PubkeyToAddress(key.PublicKey).String()

	newChain, err := w.CreateChain(keyAddr, storageConfig)
	require.Nil(t, err)

	err = w.SaveChain(newChain)
	require.Nil(t, err)

	savedChain, err := w.GetChain(newChain.MustId())
	assert.Nil(t, err)

	assert.Equal(t, newChain.Tip(), savedChain.Tip())

	origNodes, err := newChain.ChainTree.Dag.Nodes(ctx)
	require.Nil(t, err)

	savedNodes, err := savedChain.ChainTree.Dag.Nodes(ctx)
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

func SubtestWallet_SaveChain(t *testing.T, storageConfig *adapters.Config) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := NewWallet(&WalletConfig{Storage: storage.NewDefaultMemory()})
	defer w.Close()

	key, err := w.GenerateKey()
	require.Nil(t, err)

	keyAddr := crypto.PubkeyToAddress(key.PublicKey).String()

	signedTree, err := w.CreateChain(keyAddr, storageConfig)
	require.Nil(t, err)

	err = w.SaveChain(signedTree)
	require.Nil(t, err)

	savedTree, err := w.GetChain(signedTree.MustId())
	assert.Nil(t, err)
	signedNodes, err := signedTree.ChainTree.Dag.Nodes(ctx)
	require.Nil(t, err)

	savedNodes, err := savedTree.ChainTree.Dag.Nodes(ctx)
	require.Nil(t, err)
	assert.Equal(t, len(signedNodes), len(savedNodes))

	hsh := crypto.Keccak256([]byte("hi"))

	ecdsaSig, _ := crypto.Sign(hsh, key)

	sig := &signatures.Signature{
		Type:      consensus.KeyTypeSecp256k1,
		Signature: ecdsaSig,
	}

	signedTree.Signatures = consensus.SignatureMap{
		"id": sig,
	}

	err = w.SaveChain(signedTree)
	require.Nil(t, err)

	savedTree, err = w.GetChain(signedTree.MustId())
	assert.Nil(t, err)

	savedNodes, err = savedTree.ChainTree.Dag.Nodes(ctx)
	require.Nil(t, err)

	assert.Equal(t, len(signedNodes), len(savedNodes))

	txn, err := chaintree.NewSetDataTransaction("something", "hi")
	assert.Nil(t, err)
	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Transactions: []*transactions.Transaction{txn},
		},
	}

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, key)
	require.Nil(t, err)

	newTree := signedTree.ChainTree.Dag.WithNewTip(signedTree.ChainTree.Dag.Tip)

	unmarshaledRoot, err := newTree.Get(ctx, newTree.Tip)
	require.Nil(t, err)
	require.NotNil(t, unmarshaledRoot)

	root := &chaintree.RootNode{}

	err = cbornode.DecodeInto(unmarshaledRoot.RawData(), root)
	require.Nil(t, err)

	require.NotNil(t, root.Tree)

	newTree.Tip = *root.Tree

	_, err = newTree.Set(ctx, strings.Split("something", "/"), "hi")
	require.Nil(t, err)
	_, err = signedTree.ChainTree.Dag.SetAsLink(ctx, []string{chaintree.TreeLabel}, newTree.Tip)
	require.Nil(t, err)

	chainNode, err := signedTree.ChainTree.Dag.Get(ctx, *root.Chain)
	require.Nil(t, err)
	chainMap := make(map[string]interface{})
	err = cbornode.DecodeInto(chainNode.RawData(), &chainMap)
	require.Nil(t, err)

	sw := &safewrap.SafeWrap{}

	wrappedBlock := sw.WrapObject(blockWithHeaders)
	require.Nil(t, sw.Err)

	chainMap["end"] = wrappedBlock.Cid()
	newChainNode := sw.WrapObject(chainMap)

	err = signedTree.ChainTree.Dag.AddNodes(ctx, wrappedBlock)
	require.Nil(t, err)
	_, err = signedTree.ChainTree.Dag.Update(ctx, []string{chaintree.ChainLabel}, newChainNode)
	require.Nil(t, err)

	err = w.SaveChain(signedTree)
	require.Nil(t, err)

	savedTree, err = w.GetChain(signedTree.MustId())
	require.Nil(t, err)
	savedNodes, err = savedTree.ChainTree.Dag.Nodes(ctx)
	require.Nil(t, err)

	assert.Equal(t, len(signedNodes), len(savedNodes))

}

func SubtestWallet_GetChainIds(t *testing.T, storageConfig *adapters.Config) {
	w := NewWallet(&WalletConfig{Storage: storage.NewDefaultMemory()})
	defer w.Close()

	createdChains := make([]string, 3)

	for i := 0; i < 3; i++ {
		key, err := w.GenerateKey()
		require.Nil(t, err)

		keyAddr := crypto.PubkeyToAddress(key.PublicKey).String()
		signedTree, err := w.CreateChain(keyAddr, storageConfig)
		require.Nil(t, err)
		createdChains[i] = signedTree.MustId()
	}

	ids, err := w.GetChainIds()
	assert.Nil(t, err)

	assert.ElementsMatch(t, createdChains, ids)
}

func SubtestWallet_GetKey(t *testing.T, storageConfig *adapters.Config) {
	w := NewWallet(&WalletConfig{Storage: storage.NewDefaultMemory()})
	defer w.Close()

	key, err := w.GenerateKey()
	assert.Nil(t, err)

	retKey, err := w.GetKey(crypto.PubkeyToAddress(key.PublicKey).String())
	assert.Nil(t, err)
	assert.Equal(t, retKey, key)
}

func SubtestWallet_ListKeys(t *testing.T, storageConfig *adapters.Config) {
	w := NewWallet(&WalletConfig{Storage: storage.NewDefaultMemory()})
	defer w.Close()

	keyCount := 3
	// generatedKeys := make([]*ecdsa.PrivateKey, keyCount)
	addrs := make([]string, keyCount)
	for i:=0;i<keyCount;i++ {
		key, err := w.GenerateKey()
		require.Nil(t, err)
		// generatedKeys[i] = key
		addrs[i] = crypto.PubkeyToAddress(key.PublicKey).String()
	}
	keys,err := w.ListKeys()
	require.Nil(t,err)
	assert.ElementsMatch(t, addrs, keys)
}

func SubtestWallet_ChainExists(t *testing.T, storageConfig *adapters.Config) {
	w := NewWallet(&WalletConfig{Storage: storage.NewDefaultMemory()})
	defer w.Close()

	key, err := w.GenerateKey()
	require.Nil(t, err)

	keyAddr := crypto.PubkeyToAddress(key.PublicKey).String()
	chainId := consensus.EcdsaPubkeyToDid(key.PublicKey)

	assert.False(t, w.ChainExistsForKey(keyAddr))
	assert.False(t, w.ChainExists(chainId))

	signedTree, err := w.CreateChain(keyAddr, storageConfig)
	require.Nil(t, err)

	assert.True(t, w.ChainExistsForKey(keyAddr))
	assert.True(t, w.ChainExists(signedTree.MustId()))
}
