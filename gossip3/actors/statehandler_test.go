package actors

import (
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	cid "github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/quorumcontrol/tupelo-go-sdk/consensus"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/messages"
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

func TestChainTreeStateHandler(t *testing.T) {
	sw := safewrap.SafeWrap{}
	treeKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: nil,
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

	transHeight := uint64(0)

	trans := &messages.Transaction{
		State:       nodes,
		PreviousTip: emptyTree.Tip.Bytes(),
		Height:      transHeight,
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
	}

	stateTrans := &stateTransaction{
		CurrentState: nil,
		ObjectID:     []byte(treeDID),
		Transaction:  trans,
		Block:        blockWithHeaders,
	}

	validator := TransactionValidator{}
	newState, isAccepted, err := validator.chainTreeStateHandler(nil, stateTrans)
	require.Nil(t, err)
	assert.True(t, isAccepted)

	testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	_, err = testTree.ProcessBlock(blockWithHeaders)
	assert.Nil(t, err)
	assert.Equal(t, newState, testTree.Dag.Tip.Bytes())

	stateTrans2 := &stateTransaction{
		CurrentState: newState,
		ObjectID:     []byte(treeDID),
		Transaction:  trans,
		Block:        blockWithHeaders,
	}

	_, _, err = validator.chainTreeStateHandler(nil, stateTrans2)
	assert.NotNil(t, err)

	transHeight++

	// playing a new transaction should work when there are no auths
	unsignedBlock = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: &testTree.Dag.Tip,
			Height:      transHeight,
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

	trans = &messages.Transaction{
		State:       nodes,
		PreviousTip: testTree.Dag.Tip.Bytes(),
		Height:      blockWithHeaders.Height,
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
	}

	stateTrans3 := &stateTransaction{
		CurrentState: testTree.Dag.Tip.Bytes(),
		ObjectID:     []byte(treeDID),
		Transaction:  trans,
		Block:        blockWithHeaders,
	}

	newState, isAccepted, err = validator.chainTreeStateHandler(nil, stateTrans3)
	assert.Nil(t, err)
	assert.True(t, isAccepted)

	_, err = testTree.ProcessBlock(blockWithHeaders)
	assert.Nil(t, err)

	assert.Equal(t, newState, testTree.Dag.Tip.Bytes())

	newOwnerKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	newOwner := consensus.EcdsaToPublicKey(&newOwnerKey.PublicKey)
	newOwnerAddr := consensus.PublicKeyToAddr(&newOwner)

	transHeight++

	unsignedBlock = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: &testTree.Dag.Tip,
			Height:      transHeight,
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_OWNERSHIP",
					Payload: map[string]interface{}{
						"authentication": []string{
							newOwnerAddr,
						},
					},
				},
			},
		},
	}

	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)
	nodes = dagToByteNodes(t, testTree.Dag)

	trans = &messages.Transaction{
		State:       nodes,
		PreviousTip: testTree.Dag.Tip.Bytes(),
		Height:      blockWithHeaders.Height,
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
	}

	stateTrans4 := &stateTransaction{
		CurrentState: testTree.Dag.Tip.Bytes(),
		ObjectID:     []byte(treeDID),
		Transaction:  trans,
		Block:        blockWithHeaders,
	}

	newState, isAccepted, err = validator.chainTreeStateHandler(nil, stateTrans4)
	assert.Nil(t, err)
	assert.True(t, isAccepted)

	valid, err := testTree.ProcessBlock(blockWithHeaders)
	assert.True(t, valid)
	assert.Nil(t, err)

	assert.Equal(t, newState, testTree.Dag.Tip.Bytes())

	transHeight++

	// now that the owners are changed, we shouldn't be able to sign with the TreeKey

	unsignedBlock = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: &testTree.Dag.Tip,
			Height:      transHeight,
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

	trans = &messages.Transaction{
		State:       nodes,
		PreviousTip: testTree.Dag.Tip.Bytes(),
		Height:      blockWithHeaders.Height,
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
	}

	stateTrans5 := &stateTransaction{
		CurrentState: testTree.Dag.Tip.Bytes(),
		ObjectID:     []byte(treeDID),
		Transaction:  trans,
		Block:        blockWithHeaders,
	}

	_, _, err = validator.chainTreeStateHandler(nil, stateTrans5)
	assert.NotNil(t, err)

	// however if we sign it with the new owner, it should be accepted.
	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, newOwnerKey)
	assert.Nil(t, err)

	trans = &messages.Transaction{
		State:       nodes,
		PreviousTip: testTree.Dag.Tip.Bytes(),
		Height:      blockWithHeaders.Height,
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
	}

	stateTrans6 := &stateTransaction{
		CurrentState: testTree.Dag.Tip.Bytes(),
		ObjectID:     []byte(treeDID),
		Transaction:  trans,
		Block:        blockWithHeaders,
	}

	newState, isAccepted, err = validator.chainTreeStateHandler(nil, stateTrans6)
	assert.Nil(t, err)
	assert.True(t, isAccepted)

	valid, err = testTree.ProcessBlock(blockWithHeaders)
	assert.True(t, valid)
	assert.Nil(t, err)

	assert.Equal(t, newState, testTree.Dag.Tip.Bytes())

	val, _, err := testTree.Dag.Resolve([]string{"tree", "data", "another", "path"})
	assert.Nil(t, err)
	assert.Equal(t, "test", val)
}

func transToStateTrans(t *testing.T, did string, tip cid.Cid, trans *messages.Transaction) *stateTransaction {
	block := &chaintree.BlockWithHeaders{}
	err := cbornode.DecodeInto(trans.Payload, block)
	if err != nil {
		panic(fmt.Errorf("invalid block in transaction: %v", err))
	}
	return &stateTransaction{
		CurrentState: tip.Bytes(),
		Block:        block,
		Transaction:  trans,
		ObjectID:     []byte(did),
	}
}

func TestSigner_TokenTransactions(t *testing.T) {
	treeKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())

	tokenName := "testtoken"
	tokenFullName := consensus.TokenName{ChainTreeDID: treeDID, LocalName: tokenName}

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: nil,
			Height:      0,
			Transactions: []*chaintree.Transaction{
				{
					Type: "ESTABLISH_TOKEN",
					Payload: map[string]interface{}{
						"name": tokenName,
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
	require.Nil(t, err)

	sw := &safewrap.SafeWrap{}
	trans := &messages.Transaction{
		State:       nodes,
		Height:      0,
		PreviousTip: emptyTree.Tip.Bytes(),
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
	}

	validator := TransactionValidator{}
	newState, isAccepted, err := validator.chainTreeStateHandler(nil, transToStateTrans(t, treeDID, cid.Undef, trans))
	require.Nil(t, err)
	assert.True(t, isAccepted)

	testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	_, err = testTree.ProcessBlock(blockWithHeaders)
	assert.Nil(t, err)
	assert.Equal(t, newState, testTree.Dag.Tip.Bytes())

	tip, err := cid.Cast(newState)
	require.Nil(t, err)
	_, isAccepted, err = validator.chainTreeStateHandler(nil, transToStateTrans(t, treeDID, tip, trans))
	assert.NotNil(t, err)
	assert.False(t, isAccepted)

	// Can mint from established token
	for testIndex, testAmount := range []uint64{1, 30, 8000000} {
		unsignedBlock = &chaintree.BlockWithHeaders{
			Block: chaintree.Block{
				PreviousTip: &testTree.Dag.Tip,
				Height:      uint64(testIndex) + 1,
				Transactions: []*chaintree.Transaction{
					{
						Type: "MINT_TOKEN",
						Payload: map[string]interface{}{
							"name":   tokenName,
							"amount": testAmount,
						},
					},
				},
			},
		}

		nodes = dagToByteNodes(t, testTree.Dag)

		blockWithHeaders, err = consensus.SignBlock(unsignedBlock, treeKey)
		assert.Nil(t, err)

		trans = &messages.Transaction{
			State:       nodes,
			PreviousTip: testTree.Dag.Tip.Bytes(),
			Height:      1,
			Payload:     sw.WrapObject(blockWithHeaders).RawData(),
		}

		newState, isAccepted, err := validator.chainTreeStateHandler(nil, transToStateTrans(t, treeDID, testTree.Dag.Tip, trans))
		assert.Nil(t, err)
		assert.True(t, isAccepted)

		_, err = testTree.ProcessBlock(blockWithHeaders)
		assert.Nil(t, err)

		assert.Equal(t, newState, testTree.Dag.Tip.Bytes())

		mintAmount, _, err := testTree.Dag.Resolve([]string{"tree", "_tupelo", "tokens", tokenFullName.String(), consensus.TokenMintLabel, fmt.Sprint(testIndex), "amount"})
		assert.Nil(t, err)
		assert.Equal(t, mintAmount, testAmount)
	}

	// Can't mint more than the monetary policy maximum
	unsignedBlock = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: &testTree.Dag.Tip,
			Transactions: []*chaintree.Transaction{
				{
					Type: "MINT_TOKEN",
					Payload: map[string]interface{}{
						"name":   tokenName,
						"amount": 675309,
					},
				},
			},
		},
	}

	nodes = dagToByteNodes(t, testTree.Dag)

	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	trans = &messages.Transaction{
		State:       nodes,
		PreviousTip: testTree.Dag.Tip.Bytes(),
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
	}

	_, isAccepted, err = validator.chainTreeStateHandler(nil, transToStateTrans(t, treeDID, testTree.Dag.Tip, trans))
	assert.NotNil(t, err)
	assert.False(t, isAccepted)

	// Can't mint a negative amount
	unsignedBlock = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: &testTree.Dag.Tip,
			Transactions: []*chaintree.Transaction{
				{
					Type: "MINT_TOKEN",
					Payload: map[string]interface{}{
						"name":   tokenName,
						"amount": -42,
					},
				},
			},
		},
	}

	nodes = dagToByteNodes(t, testTree.Dag)

	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	trans = &messages.Transaction{
		State:       nodes,
		PreviousTip: testTree.Dag.Tip.Bytes(),
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
	}

	_, isAccepted, err = validator.chainTreeStateHandler(nil, transToStateTrans(t, treeDID, testTree.Dag.Tip, trans))
	assert.NotNil(t, err)
	assert.False(t, isAccepted)
}

func TestSigner_NextBlockValidation(t *testing.T) {
	treeKey, err := crypto.GenerateKey()
	assert.Nil(t, err)
	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())
	emptyTree := consensus.NewEmptyTree(treeDID, nodeStore)
	testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	savedcid := emptyTree.Tip

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: nil,
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

	sw := &safewrap.SafeWrap{}

	trans := &messages.Transaction{
		State:       nodes1,
		Height:      0,
		PreviousTip: testTree.Dag.Tip.Bytes(),
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
	}

	validator := TransactionValidator{}
	newState, isAccepted, err := validator.chainTreeStateHandler(nil, transToStateTrans(t, treeDID, testTree.Dag.Tip, trans))
	assert.Nil(t, err)
	assert.True(t, isAccepted)

	_, err = testTree.ProcessBlock(blockWithHeaders)
	assert.Nil(t, err)
	assert.Equal(t, newState, testTree.Dag.Tip.Bytes())

	tip, _ := cid.Cast(newState)
	unsignedBlock2 := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: &tip,
			Height:      1,
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

	trans2 := &messages.Transaction{
		State:       nodes2,
		Height:      1,
		PreviousTip: tip.Bytes(),
		Payload:     sw.WrapObject(blockWithHeaders2).RawData(),
	}

	newState, isAccepted, err = validator.chainTreeStateHandler(nil, transToStateTrans(t, treeDID, testTree.Dag.Tip, trans2))
	assert.Nil(t, err)
	assert.True(t, isAccepted)

	_, err = testTree.ProcessBlock(blockWithHeaders2)
	assert.Nil(t, err)
	assert.Equal(t, newState, testTree.Dag.Tip.Bytes())

	// test that a nil PreviousTip fails
	nodes3 := dagToByteNodes(t, testTree.Dag)

	unsignedBlock3 := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: nil,
			Height:      3,
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

	trans3 := &messages.Transaction{
		State:       nodesCombined,
		Height:      3,
		PreviousTip: savedcid.Bytes(),
		Payload:     sw.WrapObject(blockWithHeaders3).RawData(),
	}

	_, isAccepted, err = validator.chainTreeStateHandler(nil, transToStateTrans(t, treeDID, testTree.Dag.Tip, trans3))
	assert.NotNil(t, err)
	assert.False(t, isAccepted)
}
