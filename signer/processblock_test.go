package signer

import (
	"fmt"
	"testing"

	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/storage"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func dagToByteNodes(t *testing.T, dagTree *dag.Dag) [][]byte {
	cborNodes, err := dagTree.Nodes()
	require.Nil(t, err)
	nodes := make([][]byte, len(cborNodes))
	for i, node := range cborNodes {
		nodes[i] = node.RawData()
	}
	return nodes
}

func TestprocessAddBlock(t *testing.T) {

	treeKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]string{
						"path":  "down/in/the/thing",
						"value": "hi",
					},
				},
			},
		},
	}

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	emptyTree := consensus.NewEmptyTree(treeDID, nodeStore)

	nodes := dagToByteNodes(t, emptyTree)

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	req := &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      emptyTree.Tip,
		NewBlock: blockWithHeaders,
	}

	resp, err := processAddBlock(nil, req)

	assert.Nil(t, err)

	testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	testTree.ProcessBlock(blockWithHeaders)
	assert.Equal(t, resp.Tip, testTree.Dag.Tip)

	resp, err = processAddBlock(resp.Tip, req)
	assert.NotNil(t, err)

	// playing a new transaction should work when there are no auths
	unsignedBlock = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: testTree.Dag.Tip.String(),
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]string{
						"path":  "down/in/the/thing",
						"value": "hi",
					},
				},
			},
		},
	}

	nodes = dagToByteNodes(t, testTree.Dag)

	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	req = &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      testTree.Dag.Tip,
		NewBlock: blockWithHeaders,
	}

	resp, err = processAddBlock(testTree.Dag.Tip, req)
	assert.Nil(t, err)

	testTree.ProcessBlock(blockWithHeaders)

	assert.Equal(t, resp.Tip, testTree.Dag.Tip)

	newOwnerKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	newOwner := consensus.EcdsaToPublicKey(&newOwnerKey.PublicKey)

	unsignedBlock = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: testTree.Dag.Tip.String(),
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_OWNERSHIP",
					Payload: map[string]interface{}{
						"authentication": []*consensus.PublicKey{
							&newOwner,
						},
					},
				},
			},
		},
	}

	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)
	nodes = dagToByteNodes(t, testTree.Dag)
	req = &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      testTree.Dag.Tip,
		NewBlock: blockWithHeaders,
	}

	resp, err = processAddBlock(testTree.Dag.Tip, req)
	assert.Nil(t, err)

	valid, err := testTree.ProcessBlock(blockWithHeaders)
	assert.True(t, valid)
	assert.Nil(t, err)

	assert.Equal(t, resp.Tip, testTree.Dag.Tip)

	// now that the owners are changed, we shouldn't be able to sign with the TreeKey

	unsignedBlock = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: testTree.Dag.Tip.String(),
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]interface{}{
						"path":  "another/path",
						"value": "test",
					},
				},
			},
		},
	}

	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	nodes = dagToByteNodes(t, testTree.Dag)

	req = &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      testTree.Dag.Tip,
		NewBlock: blockWithHeaders,
	}

	resp, err = processAddBlock(testTree.Dag.Tip, req)
	assert.NotNil(t, err)

	// however if we sign it with the new owner, it should be accepted.
	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, newOwnerKey)
	assert.Nil(t, err)

	req = &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      testTree.Dag.Tip,
		NewBlock: blockWithHeaders,
	}

	resp, err = processAddBlock(testTree.Dag.Tip, req)
	assert.Nil(t, err)

	valid, err = testTree.ProcessBlock(blockWithHeaders)
	assert.True(t, valid)
	assert.Nil(t, err)

	assert.Equal(t, resp.Tip, testTree.Dag.Tip)

	val, _, err := testTree.Dag.Resolve([]string{"tree", "another", "path"})
	assert.Nil(t, err)
	assert.Equal(t, "test", val)

	// Should not be able to assign an authentication directly through SET_DATA
	unsignedBlock = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: testTree.Dag.Tip.String(),
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]interface{}{
						"path":  "_tupelo/authentications/publicKey",
						"value": "test",
					},
				},
			},
		},
	}

	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, newOwnerKey)
	assert.Nil(t, err)

	nodes = dagToByteNodes(t, testTree.Dag)

	req = &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      testTree.Dag.Tip,
		NewBlock: blockWithHeaders,
	}

	resp, err = processAddBlock(testTree.Dag.Tip, req)
	assert.NotNil(t, err)
}
func TestSigner_CoinTransactions(t *testing.T) {
	treeKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				{
					Type: "ESTABLISH_COIN",
					Payload: map[string]interface{}{
						"name": "testcoin",
						"monetaryPolicy": map[string]interface{}{
							"maximum": 8675309,
						},
					},
				},
			},
		},
	}

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	emptyTree := consensus.NewEmptyTree(treeDID, nodeStore)

	nodes := dagToByteNodes(t, emptyTree)

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	req := &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      emptyTree.Tip,
		NewBlock: blockWithHeaders,
	}

	resp, err := processAddBlock(nil, req)

	assert.Nil(t, err)

	testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	testTree.ProcessBlock(blockWithHeaders)
	assert.Equal(t, resp.Tip, testTree.Dag.Tip)

	resp, err = processAddBlock(resp.Tip, req)
	assert.NotNil(t, err)

	// Can mint from established coin
	for testIndex, testAmount := range []uint64{1, 30, 8000000} {
		unsignedBlock = &chaintree.BlockWithHeaders{
			Block: chaintree.Block{
				PreviousTip: testTree.Dag.Tip.String(),
				Transactions: []*chaintree.Transaction{
					{
						Type: "MINT_COIN",
						Payload: map[string]interface{}{
							"name":   "testcoin",
							"amount": testAmount,
						},
					},
				},
			},
		}

		nodes = dagToByteNodes(t, testTree.Dag)

		blockWithHeaders, err = consensus.SignBlock(unsignedBlock, treeKey)
		assert.Nil(t, err)

		req = &consensus.AddBlockRequest{
			Nodes:    nodes,
			Tip:      testTree.Dag.Tip,
			NewBlock: blockWithHeaders,
		}

		resp, err = processAddBlock(testTree.Dag.Tip, req)
		assert.Nil(t, err)

		testTree.ProcessBlock(blockWithHeaders)

		assert.Equal(t, resp.Tip, testTree.Dag.Tip)

		mintAmount, _, err := testTree.Dag.Resolve([]string{"tree", "_tupelo", "coins", "testcoin", "mints", fmt.Sprint(testIndex), "amount"})
		assert.Nil(t, err)
		assert.Equal(t, mintAmount, testAmount)
	}

	// Can't mint more than the monetary policy maximum
	unsignedBlock = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: testTree.Dag.Tip.String(),
			Transactions: []*chaintree.Transaction{
				{
					Type: "MINT_COIN",
					Payload: map[string]interface{}{
						"name":   "testcoin",
						"amount": 675309,
					},
				},
			},
		},
	}

	nodes = dagToByteNodes(t, testTree.Dag)

	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	req = &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      testTree.Dag.Tip,
		NewBlock: blockWithHeaders,
	}

	resp, err = processAddBlock(testTree.Dag.Tip, req)
	assert.NotNil(t, err)

	// Can't mint a negative amount
	unsignedBlock = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: testTree.Dag.Tip.String(),
			Transactions: []*chaintree.Transaction{
				{
					Type: "MINT_COIN",
					Payload: map[string]interface{}{
						"name":   "testcoin",
						"amount": -42,
					},
				},
			},
		},
	}

	nodes = dagToByteNodes(t, testTree.Dag)

	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	req = &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      testTree.Dag.Tip,
		NewBlock: blockWithHeaders,
	}

	resp, err = processAddBlock(testTree.Dag.Tip, req)
	assert.NotNil(t, err)
}

func TestSigner_NextBlockValidation(t *testing.T) {
	treeKey, err := crypto.GenerateKey()
	assert.Nil(t, err)
	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())
	emptyTree := consensus.NewEmptyTree(treeDID, nodeStore)
	testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)

	savedcid := *emptyTree.Tip

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]string{
						"path":  "transaction1",
						"value": "foo",
					},
				},
			},
		},
	}

	nodes1 := dagToByteNodes(t, emptyTree)

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	req := &consensus.AddBlockRequest{
		Nodes:    nodes1,
		Tip:      testTree.Dag.Tip,
		NewBlock: blockWithHeaders,
	}

	resp, err := processAddBlock(testTree.Dag.Tip, req)
	assert.Nil(t, err)

	testTree.ProcessBlock(blockWithHeaders)
	assert.Equal(t, resp.Tip, testTree.Dag.Tip)

	unsignedBlock2 := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: resp.Tip.String(),
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]string{
						"path":  "transaction2",
						"value": "bar",
					},
				},
			},
		},
	}

	nodes2 := dagToByteNodes(t, testTree.Dag)

	blockWithHeaders2, err := consensus.SignBlock(unsignedBlock2, treeKey)
	assert.Nil(t, err)

	req2 := &consensus.AddBlockRequest{
		Nodes:    nodes2,
		Tip:      resp.Tip,
		NewBlock: blockWithHeaders2,
	}

	resp2, err := processAddBlock(testTree.Dag.Tip, req2)
	assert.Nil(t, err)

	testTree.ProcessBlock(blockWithHeaders2)
	assert.Equal(t, resp2.Tip, testTree.Dag.Tip)

	// test that a nil PreviousTip fails
	nodes3 := dagToByteNodes(t, testTree.Dag)

	unsignedBlock3 := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]string{
						"path":  "transaction1",
						"value": "foo",
					},
				},
			},
		},
	}

	blockWithHeaders3, err := consensus.SignBlock(unsignedBlock3, treeKey)
	assert.Nil(t, err)

	nodesCombined := append(append(nodes1, nodes2...), nodes3...)

	req3 := &consensus.AddBlockRequest{
		Nodes:    nodesCombined,
		Tip:      &savedcid,
		NewBlock: blockWithHeaders3,
	}

	_, err = processAddBlock(testTree.Dag.Tip, req3)
	assert.NotNil(t, err)
}
