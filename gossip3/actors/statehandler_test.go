package actors

import (
	"context"
	"fmt"
	"testing"

	"github.com/quorumcontrol/messages/build/go/services"

	"github.com/ethereum/go-ethereum/crypto"
	cid "github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/build/go/transactions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/quorumcontrol/tupelo-go-sdk/consensus"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
)

func dagToByteNodes(t *testing.T, dagTree *dag.Dag) [][]byte {
	cborNodes, err := dagTree.Nodes(context.TODO())
	require.Nil(t, err)
	nodes := make([][]byte, len(cborNodes))
	for i, node := range cborNodes {
		nodes[i] = node.RawData()
	}
	return nodes
}

func newTransactionValidator() TransactionValidator {
	return TransactionValidator{
		notaryGroup: types.NewNotaryGroup("testnotarygroup"),
	}
}

func TestChainTreeStateHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sw := safewrap.SafeWrap{}
	treeKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())

	txn, err := chaintree.NewSetDataTransaction("down/in/the/thing", "hi")
	assert.Nil(t, err)

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Transactions: []*transactions.Transaction{txn},
		},
	}

	nodeStore := nodestore.MustMemoryStore(ctx)
	emptyTree := consensus.NewEmptyTree(treeDID, nodeStore)

	nodes := dagToByteNodes(t, emptyTree)

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	transHeight := uint64(0)

	trans := &services.AddBlockRequest{
		State:       nodes,
		PreviousTip: emptyTree.Tip.Bytes(),
		Height:      transHeight,
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
	}

	stateTrans := &stateTransaction{
		CurrentState: nil,
		ObjectId:     []byte(treeDID),
		Transaction:  trans,
		Block:        blockWithHeaders,
	}

	validator := newTransactionValidator()
	newState, isAccepted, err := validator.chainTreeStateHandler(nil, stateTrans)
	require.Nil(t, err)
	assert.True(t, isAccepted)

	testTree, err := chaintree.NewChainTree(ctx, emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	_, err = testTree.ProcessBlock(ctx, blockWithHeaders)
	assert.Nil(t, err)
	assert.Equal(t, newState, testTree.Dag.Tip.Bytes())

	stateTrans2 := &stateTransaction{
		CurrentState: newState,
		ObjectId:     []byte(treeDID),
		Transaction:  trans,
		Block:        blockWithHeaders,
	}

	_, _, err = validator.chainTreeStateHandler(nil, stateTrans2)
	assert.NotNil(t, err)

	transHeight++

	// playing a new transaction should work when there are no auths
	txn2, err := chaintree.NewSetDataTransaction("down/in/the/thing", "hi")
	assert.Nil(t, err)

	unsignedBlock = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &testTree.Dag.Tip,
			Height:       transHeight,
			Transactions: []*transactions.Transaction{txn2},
		},
	}

	nodes = dagToByteNodes(t, testTree.Dag)

	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	trans = &services.AddBlockRequest{
		State:       nodes,
		PreviousTip: testTree.Dag.Tip.Bytes(),
		Height:      blockWithHeaders.Height,
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
	}

	stateTrans3 := &stateTransaction{
		CurrentState: testTree.Dag.Tip.Bytes(),
		ObjectId:     []byte(treeDID),
		Transaction:  trans,
		Block:        blockWithHeaders,
	}

	newState, isAccepted, err = validator.chainTreeStateHandler(nil, stateTrans3)
	assert.Nil(t, err)
	assert.True(t, isAccepted)

	_, err = testTree.ProcessBlock(ctx, blockWithHeaders)
	assert.Nil(t, err)

	assert.Equal(t, newState, testTree.Dag.Tip.Bytes())

	newOwnerKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	newOwner := consensus.EcdsaToPublicKey(&newOwnerKey.PublicKey)
	newOwnerAddr := consensus.PublicKeyToAddr(&newOwner)

	transHeight++

	setOwnerTxn, err := chaintree.NewSetOwnershipTransaction([]string{newOwnerAddr})
	assert.Nil(t, err)
	unsignedBlock = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &testTree.Dag.Tip,
			Height:       transHeight,
			Transactions: []*transactions.Transaction{setOwnerTxn},
		},
	}

	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)
	nodes = dagToByteNodes(t, testTree.Dag)

	trans = &services.AddBlockRequest{
		State:       nodes,
		PreviousTip: testTree.Dag.Tip.Bytes(),
		Height:      blockWithHeaders.Height,
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
	}

	stateTrans4 := &stateTransaction{
		CurrentState: testTree.Dag.Tip.Bytes(),
		ObjectId:     []byte(treeDID),
		Transaction:  trans,
		Block:        blockWithHeaders,
	}

	newState, isAccepted, err = validator.chainTreeStateHandler(nil, stateTrans4)
	assert.Nil(t, err)
	assert.True(t, isAccepted)

	valid, err := testTree.ProcessBlock(ctx, blockWithHeaders)
	assert.True(t, valid)
	assert.Nil(t, err)

	assert.Equal(t, newState, testTree.Dag.Tip.Bytes())

	transHeight++

	// now that the owners are changed, we shouldn't be able to sign with the TreeKey
	setDataTxn2, err := chaintree.NewSetDataTransaction("another/path", "test")
	assert.Nil(t, err)

	unsignedBlock = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &testTree.Dag.Tip,
			Height:       transHeight,
			Transactions: []*transactions.Transaction{setDataTxn2},
		},
	}

	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	nodes = dagToByteNodes(t, testTree.Dag)

	trans = &services.AddBlockRequest{
		State:       nodes,
		PreviousTip: testTree.Dag.Tip.Bytes(),
		Height:      blockWithHeaders.Height,
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
	}

	stateTrans5 := &stateTransaction{
		CurrentState: testTree.Dag.Tip.Bytes(),
		ObjectId:     []byte(treeDID),
		Transaction:  trans,
		Block:        blockWithHeaders,
	}

	_, _, err = validator.chainTreeStateHandler(nil, stateTrans5)
	assert.NotNil(t, err)

	// however if we sign it with the new owner, it should be accepted.
	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, newOwnerKey)
	assert.Nil(t, err)

	trans = &services.AddBlockRequest{
		State:       nodes,
		PreviousTip: testTree.Dag.Tip.Bytes(),
		Height:      blockWithHeaders.Height,
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
	}

	stateTrans6 := &stateTransaction{
		CurrentState: testTree.Dag.Tip.Bytes(),
		ObjectId:     []byte(treeDID),
		Transaction:  trans,
		Block:        blockWithHeaders,
	}

	newState, isAccepted, err = validator.chainTreeStateHandler(nil, stateTrans6)
	assert.Nil(t, err)
	assert.True(t, isAccepted)

	valid, err = testTree.ProcessBlock(ctx, blockWithHeaders)
	assert.True(t, valid)
	assert.Nil(t, err)

	assert.Equal(t, newState, testTree.Dag.Tip.Bytes())

	val, _, err := testTree.Dag.Resolve(ctx, []string{"tree", "data", "another", "path"})
	assert.Nil(t, err)
	assert.Equal(t, "test", val)
}

func transToStateTrans(t *testing.T, did string, tip cid.Cid, trans *services.AddBlockRequest) *stateTransaction {
	block := &chaintree.BlockWithHeaders{}
	err := cbornode.DecodeInto(trans.Payload, block)
	if err != nil {
		panic(fmt.Errorf("invalid block in transaction: %v", err))
	}
	return &stateTransaction{
		CurrentState: tip.Bytes(),
		Block:        block,
		Transaction:  trans,
		ObjectId:     []byte(did),
	}
}

func TestSigner_TokenTransactions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	treeKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())

	tokenName := "testtoken"
	tokenFullName := consensus.TokenName{ChainTreeDID: treeDID, LocalName: tokenName}

	establishTxn, err := chaintree.NewEstablishTokenTransaction(tokenName, 8675309)
	assert.Nil(t, err)

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       0,
			Transactions: []*transactions.Transaction{establishTxn},
		},
	}

	nodeStore := nodestore.MustMemoryStore(ctx)
	emptyTree := consensus.NewEmptyTree(treeDID, nodeStore)

	nodes := dagToByteNodes(t, emptyTree)

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	require.Nil(t, err)

	sw := &safewrap.SafeWrap{}
	trans := &services.AddBlockRequest{
		State:       nodes,
		Height:      0,
		PreviousTip: emptyTree.Tip.Bytes(),
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
	}

	validator := newTransactionValidator()
	newState, isAccepted, err := validator.chainTreeStateHandler(nil, transToStateTrans(t, treeDID, cid.Undef, trans))
	require.Nil(t, err)
	assert.True(t, isAccepted)

	testTree, err := chaintree.NewChainTree(ctx, emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	_, err = testTree.ProcessBlock(ctx, blockWithHeaders)
	assert.Nil(t, err)
	assert.Equal(t, newState, testTree.Dag.Tip.Bytes())

	tip, err := cid.Cast(newState)
	require.Nil(t, err)
	_, isAccepted, err = validator.chainTreeStateHandler(nil, transToStateTrans(t, treeDID, tip, trans))
	assert.NotNil(t, err)
	assert.False(t, isAccepted)

	// Can mint from established token
	for testIndex, testAmount := range []uint64{1, 30, 8000000} {
		mintTxn, err := chaintree.NewMintTokenTransaction(tokenName, testAmount)
		assert.Nil(t, err)

		unsignedBlock = &chaintree.BlockWithHeaders{
			Block: chaintree.Block{
				PreviousTip:  &testTree.Dag.Tip,
				Height:       uint64(testIndex) + 1,
				Transactions: []*transactions.Transaction{mintTxn},
			},
		}

		nodes = dagToByteNodes(t, testTree.Dag)

		blockWithHeaders, err = consensus.SignBlock(unsignedBlock, treeKey)
		assert.Nil(t, err)

		trans = &services.AddBlockRequest{
			State:       nodes,
			PreviousTip: testTree.Dag.Tip.Bytes(),
			Height:      1,
			Payload:     sw.WrapObject(blockWithHeaders).RawData(),
		}

		newState, isAccepted, err := validator.chainTreeStateHandler(nil, transToStateTrans(t, treeDID, testTree.Dag.Tip, trans))
		assert.Nil(t, err)
		assert.True(t, isAccepted)

		_, err = testTree.ProcessBlock(ctx, blockWithHeaders)
		assert.Nil(t, err)

		assert.Equal(t, newState, testTree.Dag.Tip.Bytes())

		mintAmount, _, err := testTree.Dag.Resolve(ctx, []string{"tree", "_tupelo", "tokens", tokenFullName.String(), consensus.TokenMintLabel, fmt.Sprint(testIndex), "amount"})
		assert.Nil(t, err)
		assert.Equal(t, mintAmount, testAmount)
	}

	// Can't mint more than the monetary policy maximum
	newMintTxn, err := chaintree.NewMintTokenTransaction(tokenName, 675309)
	assert.Nil(t, err)

	unsignedBlock = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &testTree.Dag.Tip,
			Transactions: []*transactions.Transaction{newMintTxn},
		},
	}

	nodes = dagToByteNodes(t, testTree.Dag)

	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	trans = &services.AddBlockRequest{
		State:       nodes,
		PreviousTip: testTree.Dag.Tip.Bytes(),
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
	}

	_, isAccepted, err = validator.chainTreeStateHandler(nil, transToStateTrans(t, treeDID, testTree.Dag.Tip, trans))
	assert.NotNil(t, err)
	assert.False(t, isAccepted)

	// TODO: remove commented test block. This test block is commented
	// because it is possibly no longer necessary. Attempting to create a
	// transaction with a negative amount results in a type error

	// Can't mint a negative amount
	// negMintTxn, err := chaintree.NewMintTokenTransaction(tokenName, -42)
	// assert.Nil(t, err)

	// unsignedBlock = &chaintree.BlockWithHeaders{
	//	Block: chaintree.Block{
	//		PreviousTip:  &testTree.Dag.Tip,
	//		Transactions: []*transactions.Transaction{newMintTxn},
	//	},
	// }

	// nodes = dagToByteNodes(t, testTree.Dag)

	// blockWithHeaders, err = consensus.SignBlock(unsignedBlock, treeKey)
	// assert.Nil(t, err)

	// trans = &services.AddBlockRequest{
	//	State:       nodes,
	//	PreviousTip: testTree.Dag.Tip.Bytes(),
	//	Payload:     sw.WrapObject(blockWithHeaders).RawData(),
	// }

	// _, isAccepted, err = validator.chainTreeStateHandler(nil, transToStateTrans(t, treeDID, testTree.Dag.Tip, trans))
	// assert.NotNil(t, err)
	// assert.False(t, isAccepted)
}

func TestSigner_NextBlockValidation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	treeKey, err := crypto.GenerateKey()
	assert.Nil(t, err)
	nodeStore := nodestore.MustMemoryStore(ctx)
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())
	emptyTree := consensus.NewEmptyTree(treeDID, nodeStore)
	testTree, err := chaintree.NewChainTree(ctx, emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	savedcid := emptyTree.Tip

	setDataTxn, err := chaintree.NewSetDataTransaction("transaction1", "foo")
	assert.Nil(t, err)
	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Transactions: []*transactions.Transaction{setDataTxn},
		},
	}

	nodes1 := dagToByteNodes(t, emptyTree)

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	sw := &safewrap.SafeWrap{}

	trans := &services.AddBlockRequest{
		State:       nodes1,
		Height:      0,
		PreviousTip: testTree.Dag.Tip.Bytes(),
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
	}

	validator := newTransactionValidator()
	newState, isAccepted, err := validator.chainTreeStateHandler(nil, transToStateTrans(t, treeDID, testTree.Dag.Tip, trans))
	assert.Nil(t, err)
	assert.True(t, isAccepted)

	_, err = testTree.ProcessBlock(ctx, blockWithHeaders)
	assert.Nil(t, err)
	assert.Equal(t, newState, testTree.Dag.Tip.Bytes())

	tip, _ := cid.Cast(newState)

	setDataTxn2, err := chaintree.NewSetDataTransaction("transaction2", "bar")
	assert.Nil(t, err)

	unsignedBlock2 := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &tip,
			Height:       1,
			Transactions: []*transactions.Transaction{setDataTxn2},
		},
	}

	nodes2 := dagToByteNodes(t, testTree.Dag)

	blockWithHeaders2, err := consensus.SignBlock(unsignedBlock2, treeKey)
	assert.Nil(t, err)

	trans2 := &services.AddBlockRequest{
		State:       nodes2,
		Height:      1,
		PreviousTip: tip.Bytes(),
		Payload:     sw.WrapObject(blockWithHeaders2).RawData(),
	}

	newState, isAccepted, err = validator.chainTreeStateHandler(nil, transToStateTrans(t, treeDID, testTree.Dag.Tip, trans2))
	assert.Nil(t, err)
	assert.True(t, isAccepted)

	_, err = testTree.ProcessBlock(ctx, blockWithHeaders2)
	assert.Nil(t, err)
	assert.Equal(t, newState, testTree.Dag.Tip.Bytes())

	// test that a nil PreviousTip fails
	nodes3 := dagToByteNodes(t, testTree.Dag)

	setDataTxn3, err := chaintree.NewSetDataTransaction("transaction1", "foo")
	assert.Nil(t, err)

	unsignedBlock3 := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       3,
			Transactions: []*transactions.Transaction{setDataTxn3},
		},
	}

	blockWithHeaders3, err := consensus.SignBlock(unsignedBlock3, treeKey)
	assert.Nil(t, err)

	nodesCombined := append(append(nodes1, nodes2...), nodes3...)

	trans3 := &services.AddBlockRequest{
		State:       nodesCombined,
		Height:      3,
		PreviousTip: savedcid.Bytes(),
		Payload:     sw.WrapObject(blockWithHeaders3).RawData(),
	}

	_, isAccepted, err = validator.chainTreeStateHandler(nil, transToStateTrans(t, treeDID, testTree.Dag.Tip, trans3))
	assert.NotNil(t, err)
	assert.False(t, isAccepted)
}
