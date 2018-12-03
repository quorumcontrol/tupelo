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

	maximum, _, err := testTree.Dag.Resolve([]string{"tree", "_tupelo", "coins", "testcoin", "monetaryPolicy", "maximum"})
	assert.Nil(t, err)
	assert.Equal(t, maximum, uint64(42))

	mints, _, err := testTree.Dag.Resolve([]string{"tree", "_tupelo", "coins", "testcoin", "mints"})
	assert.Nil(t, err)
	assert.Nil(t, mints)

	sends, _, err := testTree.Dag.Resolve([]string{"tree", "_tupelo", "coins", "testcoin", "sends"})
	assert.Nil(t, err)
	assert.Nil(t, sends)

	receives, _, err := testTree.Dag.Resolve([]string{"tree", "_tupelo", "coins", "testcoin", "receives"})
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

	maximum, _, err := testTree.Dag.Resolve([]string{"tree", "_tupelo", "coins", "testcoin", "monetaryPolicy", "maximum"})
	assert.Nil(t, err)
	assert.Empty(t, maximum)

	mints, _, err := testTree.Dag.Resolve([]string{"tree", "_tupelo", "coins", "testcoin", "mints"})
	assert.Nil(t, err)
	assert.Nil(t, mints)

	sends, _, err := testTree.Dag.Resolve([]string{"tree", "_tupelo", "coins", "testcoin", "sends"})
	assert.Nil(t, err)
	assert.Nil(t, sends)

	receives, _, err := testTree.Dag.Resolve([]string{"tree", "_tupelo", "coins", "testcoin", "receives"})
	assert.Nil(t, err)
	assert.Nil(t, receives)
}

func TestSendCoin(t *testing.T) {
	key, err := crypto.GenerateKey()
	assert.Nil(t, err)

	store := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	treeDID := AddrToDid(crypto.PubkeyToAddress(key.PublicKey).String())
	emptyTree := NewEmptyTree(treeDID, store)

	maximumAmount := uint64(50)
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
				{
					Type: "MINT_COIN",
					Payload: map[string]interface{}{
						"name":   "testcoin",
						"amount": maximumAmount,
					},
				},
			},
		},
	}

	testTree, err := chaintree.NewChainTree(emptyTree, nil, DefaultTransactors)
	assert.Nil(t, err)

	_, err = testTree.ProcessBlock(blockWithHeaders)
	assert.Nil(t, err)

	targetKey, err := crypto.GenerateKey()
	targetTreeDID := AddrToDid(crypto.PubkeyToAddress(targetKey.PublicKey).String())

	sendBlockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: testTree.Dag.Tip.String(),
			Transactions: []*chaintree.Transaction{
				{
					Type: "SEND_COIN",
					Payload: map[string]interface{}{
						"id":          "1234",
						"name":        "testcoin",
						"amount":      30,
						"destination": targetTreeDID,
					},
				},
			},
		},
	}

	_, err = testTree.ProcessBlock(sendBlockWithHeaders)
	assert.Nil(t, err)

	sends, _, err := testTree.Dag.Resolve([]string{"tree", "_tupelo", "coins", "testcoin", "sends", "0"})
	assert.Nil(t, err)
	assert.NotNil(t, sends)

	sendsMap := sends.(map[string]interface{})
	assert.Equal(t, sendsMap["id"], "1234")
	assert.Equal(t, sendsMap["amount"], uint64(30))
	lastSendAmount := sendsMap["amount"].(uint64)
	assert.Equal(t, sendsMap["destination"], targetTreeDID)

	overSpendBlockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: testTree.Dag.Tip.String(),
			Transactions: []*chaintree.Transaction{
				{
					Type: "SEND_COIN",
					Payload: map[string]interface{}{
						"id":          "1234",
						"name":        "testcoin",
						"amount":      (maximumAmount - lastSendAmount) + 1,
						"destination": targetTreeDID,
					},
				},
			},
		},
	}

	_, err = testTree.ProcessBlock(overSpendBlockWithHeaders)
	assert.NotNil(t, err)
}
