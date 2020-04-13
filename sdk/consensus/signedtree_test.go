package consensus

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/messages/v2/build/go/transactions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignedChainTree_IsGenesis(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key, err := crypto.GenerateKey()
	require.Nil(t, err)
	nodeStore := nodestore.MustMemoryStore(context.TODO())

	newTree, err := NewSignedChainTree(ctx, key.PublicKey, nodeStore)
	require.Nil(t, err)

	require.True(t, newTree.IsGenesis())

	txn, err := chaintree.NewSetDataTransaction("test", "value")
	assert.Nil(t, err)

	unsignedBlock := chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       0,
			Transactions: []*transactions.Transaction{txn},
		},
	}

	blockWithHeaders, err := SignBlock(ctx, &unsignedBlock, key)
	require.Nil(t, err)

	isValid, err := newTree.ChainTree.ProcessBlock(context.TODO(), blockWithHeaders)
	require.Nil(t, err)
	require.True(t, isValid)

	require.False(t, newTree.IsGenesis())

}

func TestSignedChainTree_Authentications(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key, err := crypto.GenerateKey()
	require.Nil(t, err)
	nodeStore := nodestore.MustMemoryStore(context.TODO())

	newTree, err := NewSignedChainTree(ctx, key.PublicKey, nodeStore)
	require.Nil(t, err)

	auths, err := newTree.Authentications()
	require.Nil(t, err)
	addr := crypto.PubkeyToAddress(key.PublicKey).String()
	require.Equal(t, auths, []string{addr})

	// Change key and test addrs
	newKey, err := crypto.GenerateKey()
	require.Nil(t, err)
	newAddr := crypto.PubkeyToAddress(newKey.PublicKey).String()

	txn, err := chaintree.NewSetOwnershipTransaction([]string{newAddr})
	assert.Nil(t, err)

	unsignedBlock := chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       0,
			Transactions: []*transactions.Transaction{txn},
		},
	}

	blockWithHeaders, err := SignBlock(ctx, &unsignedBlock, newKey)
	require.Nil(t, err)

	isValid, err := newTree.ChainTree.ProcessBlock(ctx, blockWithHeaders)
	require.Nil(t, err)
	require.True(t, isValid)

	newAuths, err := newTree.Authentications()
	require.Nil(t, err)
	require.Equal(t, newAuths, []string{newAddr})
}
