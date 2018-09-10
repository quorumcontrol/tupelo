package consensus

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/storage"
	"github.com/stretchr/testify/assert"
)

func TestEstablishCoinTransactionWithMaximum(t *testing.T) {
	key, err := crypto.GenerateKey()
	assert.Nil(t, err)

	store := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	treeDID := AddrToDid(crypto.PubkeyToAddress(key.PublicKey).String())
	emptyTree := NewEmptyTree(treeDID, store)

	blockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				{
					Type: "ESTABLISH_COIN",
					Payload: map[string]interface{}{
						"name": "testcoin",
						"monetaryPolicy": map[string]interface{}{
							"maximum": 42,
						},
					},
				},
			},
		},
	}

	testTree, err := chaintree.NewChainTree(emptyTree, nil, DefaultTransactors)
	assert.Nil(t, err)

	_, err = testTree.ProcessBlock(blockWithHeaders)
	assert.Nil(t, err)

	maximum, _, err := testTree.Dag.Resolve([]string{"tree", "_qc", "coins", "testcoin", "monetaryPolicy", "maximum"})
	assert.Nil(t, err)
	assert.Equal(t, maximum, uint64(42))

	mints, _, err := testTree.Dag.Resolve([]string{"tree", "_qc", "coins", "testcoin", "mints"})
	assert.Nil(t, err)
	assert.Nil(t, mints)

	sends, _, err := testTree.Dag.Resolve([]string{"tree", "_qc", "coins", "testcoin", "sends"})
	assert.Nil(t, err)
	assert.Nil(t, sends)

	receives, _, err := testTree.Dag.Resolve([]string{"tree", "_qc", "coins", "testcoin", "receives"})
	assert.Nil(t, err)
	assert.Nil(t, receives)
}

func TestEstablishCoinTransactionWithoutMonetaryPolicy(t *testing.T) {
	key, err := crypto.GenerateKey()
	assert.Nil(t, err)

	store := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	treeDID := AddrToDid(crypto.PubkeyToAddress(key.PublicKey).String())
	emptyTree := NewEmptyTree(treeDID, store)

	blockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				{
					Type: "ESTABLISH_COIN",
					Payload: map[string]interface{}{
						"name": "testcoin",
					},
				},
			},
		},
	}

	testTree, err := chaintree.NewChainTree(emptyTree, nil, DefaultTransactors)
	assert.Nil(t, err)

	_, err = testTree.ProcessBlock(blockWithHeaders)
	assert.Nil(t, err)

	maximum, _, err := testTree.Dag.Resolve([]string{"tree", "_qc", "coins", "testcoin", "monetaryPolicy", "maximum"})
	assert.Nil(t, err)
	assert.Empty(t, maximum)

	mints, _, err := testTree.Dag.Resolve([]string{"tree", "_qc", "coins", "testcoin", "mints"})
	assert.Nil(t, err)
	assert.Nil(t, mints)

	sends, _, err := testTree.Dag.Resolve([]string{"tree", "_qc", "coins", "testcoin", "sends"})
	assert.Nil(t, err)
	assert.Nil(t, sends)

	receives, _, err := testTree.Dag.Resolve([]string{"tree", "_qc", "coins", "testcoin", "receives"})
	assert.Nil(t, err)
	assert.Nil(t, receives)
}
