package consensus_test

import (
	"context"
	"strings"
	"testing"

	"github.com/quorumcontrol/messages/v2/build/go/gossip"
	"github.com/quorumcontrol/tupelo/sdk/consensus"
	"github.com/quorumcontrol/tupelo/sdk/gossip/types"

	"github.com/ethereum/go-ethereum/crypto"
	cid "github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/messages/v2/build/go/transactions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/quorumcontrol/chaintree/typecaster"
)

func TestEstablishTokenTransactionWithMaximum(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key, err := crypto.GenerateKey()
	assert.Nil(t, err)

	store := nodestore.MustMemoryStore(context.TODO())
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(key.PublicKey).String())
	emptyTree := consensus.NewEmptyTree(ctx, treeDID, store)

	tokenName := "testtoken"
	tokenFullName := strings.Join([]string{treeDID, tokenName}, ":")

	txn, err := chaintree.NewEstablishTokenTransaction(tokenName, 42)
	assert.Nil(t, err)

	blockWithHeaders := chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       0,
			Transactions: []*transactions.Transaction{txn},
		},
	}

	testTree, err := chaintree.NewChainTree(context.TODO(), emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	_, err = testTree.ProcessBlock(context.TODO(), &blockWithHeaders)
	assert.Nil(t, err)

	monetaryPolicy := transactions.TokenMonetaryPolicy{}
	err = testTree.Dag.ResolveInto(context.TODO(), []string{"tree", "_tupelo", "tokens", tokenFullName, "monetaryPolicy"}, &monetaryPolicy)

	assert.Nil(t, err)
	assert.Equal(t, monetaryPolicy.Maximum, uint64(42))

	mints, _, err := testTree.Dag.Resolve(context.TODO(), []string{"tree", "_tupelo", "tokens", tokenFullName, "mints"})
	assert.Nil(t, err)
	assert.Nil(t, mints)

	sends, _, err := testTree.Dag.Resolve(context.TODO(), []string{"tree", "_tupelo", "tokens", tokenFullName, "sends"})
	assert.Nil(t, err)
	assert.Nil(t, sends)

	receives, _, err := testTree.Dag.Resolve(context.TODO(), []string{"tree", "_tupelo", "tokens", tokenFullName, "receives"})
	assert.Nil(t, err)
	assert.Nil(t, receives)
}

func TestMintToken(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key, err := crypto.GenerateKey()
	assert.Nil(t, err)

	store := nodestore.MustMemoryStore(context.TODO())
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(key.PublicKey).String())
	emptyTree := consensus.NewEmptyTree(ctx, treeDID, store)

	tokenName := "testtoken"
	tokenFullName := strings.Join([]string{treeDID, tokenName}, ":")

	txn, err := chaintree.NewEstablishTokenTransaction(tokenName, 42)
	assert.Nil(t, err)

	blockWithHeaders := chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       0,
			Transactions: []*transactions.Transaction{txn},
		},
	}

	testTree, err := chaintree.NewChainTree(context.TODO(), emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	_, err = testTree.ProcessBlock(context.TODO(), &blockWithHeaders)
	assert.Nil(t, err)

	mintTxn, err := chaintree.NewMintTokenTransaction(tokenName, 40)
	assert.Nil(t, err)

	mintBlockWithHeaders := chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &testTree.Dag.Tip,
			Height:       1,
			Transactions: []*transactions.Transaction{mintTxn},
		},
	}
	_, err = testTree.ProcessBlock(context.TODO(), &mintBlockWithHeaders)
	assert.Nil(t, err)

	mints, _, err := testTree.Dag.Resolve(context.TODO(), []string{"tree", "_tupelo", "tokens", tokenFullName, "mints"})
	assert.Nil(t, err)
	assert.Len(t, mints.([]interface{}), 1)

	// a 2nd mint succeeds when it's within the bounds of the maximum

	mintTxn2, err := chaintree.NewMintTokenTransaction(tokenName, 1)
	assert.Nil(t, err)

	mintBlockWithHeaders2 := chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &testTree.Dag.Tip,
			Height:       2,
			Transactions: []*transactions.Transaction{mintTxn2},
		},
	}
	_, err = testTree.ProcessBlock(context.TODO(), &mintBlockWithHeaders2)
	assert.Nil(t, err)

	mints, _, err = testTree.Dag.Resolve(context.TODO(), []string{"tree", "_tupelo", "tokens", tokenFullName, "mints"})
	assert.Nil(t, err)
	assert.Len(t, mints.([]interface{}), 2)

	// a third mint fails if it exceeds the max

	mintTxn3, err := chaintree.NewMintTokenTransaction(tokenName, 100)
	assert.Nil(t, err)

	mintBlockWithHeaders3 := chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &testTree.Dag.Tip,
			Height:       0,
			Transactions: []*transactions.Transaction{mintTxn3},
		},
	}
	_, err = testTree.ProcessBlock(context.TODO(), &mintBlockWithHeaders3)
	assert.NotNil(t, err)

	mints, _, err = testTree.Dag.Resolve(context.TODO(), []string{"tree", "_tupelo", "tokens", tokenFullName, "mints"})
	assert.Nil(t, err)
	assert.Len(t, mints.([]interface{}), 2)
}

func TestEstablishTokenTransactionWithoutMonetaryPolicy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key, err := crypto.GenerateKey()
	assert.Nil(t, err)

	store := nodestore.MustMemoryStore(context.TODO())
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(key.PublicKey).String())
	emptyTree := consensus.NewEmptyTree(ctx, treeDID, store)

	tokenName := "testtoken"
	tokenFullName := strings.Join([]string{treeDID, tokenName}, ":")

	payload := &transactions.EstablishTokenPayload{Name: tokenName}

	txn := &transactions.Transaction{
		Type:                  transactions.Transaction_ESTABLISHTOKEN,
		EstablishTokenPayload: payload,
	}

	blockWithHeaders := chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       0,
			Transactions: []*transactions.Transaction{txn},
		},
	}

	testTree, err := chaintree.NewChainTree(context.TODO(), emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	_, err = testTree.ProcessBlock(context.TODO(), &blockWithHeaders)
	assert.Nil(t, err)

	maximum, _, err := testTree.Dag.Resolve(context.TODO(), []string{"tree", "_tupelo", "tokens", tokenFullName, "monetaryPolicy", "maximum"})
	assert.Nil(t, err)
	assert.Empty(t, maximum)

	mints, _, err := testTree.Dag.Resolve(context.TODO(), []string{"tree", "_tupelo", "tokens", tokenFullName, "mints"})
	assert.Nil(t, err)
	assert.Nil(t, mints)

	sends, _, err := testTree.Dag.Resolve(context.TODO(), []string{"tree", "_tupelo", "tokens", tokenFullName, "sends"})
	assert.Nil(t, err)
	assert.Nil(t, sends)

	receives, _, err := testTree.Dag.Resolve(context.TODO(), []string{"tree", "_tupelo", "tokens", tokenFullName, "receives"})
	assert.Nil(t, err)
	assert.Nil(t, receives)
}

func TestSetData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	treeKey, err := crypto.GenerateKey()
	require.Nil(t, err)
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())
	nodeStore := nodestore.MustMemoryStore(context.TODO())
	emptyTree := consensus.NewEmptyTree(ctx, treeDID, nodeStore)
	path := "some/data"
	value := "is now set"

	txn, err := chaintree.NewSetDataTransaction(path, value)
	assert.Nil(t, err)

	unsignedBlock := chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       0,
			Transactions: []*transactions.Transaction{txn},
		},
	}
	testTree, err := chaintree.NewChainTree(context.TODO(), emptyTree, nil, consensus.DefaultTransactors)
	require.Nil(t, err)

	blockWithHeaders, err := consensus.SignBlock(ctx, &unsignedBlock, treeKey)
	require.Nil(t, err)

	_, err = testTree.ProcessBlock(ctx, blockWithHeaders)
	require.Nil(t, err)

	// nodes, err := testTree.Dag.Nodes()
	// require.Nil(t, err)

	// assert the node the tree links to isn't itself a CID
	root, err := testTree.Dag.Get(ctx, testTree.Dag.Tip)
	assert.Nil(t, err)
	rootMap := make(map[string]interface{})
	err = cbornode.DecodeInto(root.RawData(), &rootMap)
	require.Nil(t, err)
	linkedTree, err := testTree.Dag.Get(ctx, rootMap["tree"].(cid.Cid))
	require.Nil(t, err)

	// assert that the thing being linked to isn't a CID itself
	treeCid := cid.Cid{}
	err = cbornode.DecodeInto(linkedTree.RawData(), &treeCid)
	assert.NotNil(t, err)

	// assert the thing being linked to is a map with data key
	treeMap := make(map[string]interface{})
	err = cbornode.DecodeInto(linkedTree.RawData(), &treeMap)
	assert.Nil(t, err)
	dataCid, ok := treeMap["data"]
	assert.True(t, ok)

	// assert the thing being linked to is a map with actual set data
	dataTree, err := testTree.Dag.Get(ctx, dataCid.(cid.Cid))
	assert.Nil(t, err)
	dataMap := make(map[string]interface{})
	err = cbornode.DecodeInto(dataTree.RawData(), &dataMap)
	assert.Nil(t, err)
	_, ok = dataMap["some"]
	assert.True(t, ok)

	// make sure the original data is still there after setting new data
	path = "other/data"
	value = "is also set"

	txn, err = chaintree.NewSetDataTransaction(path, value)
	assert.Nil(t, err)

	unsignedBlock = chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			Height:       1,
			PreviousTip:  &testTree.Dag.Tip,
			Transactions: []*transactions.Transaction{txn},
		},
	}

	blockWithHeaders, err = consensus.SignBlock(ctx, &unsignedBlock, treeKey)
	require.Nil(t, err)

	_, err = testTree.ProcessBlock(ctx, blockWithHeaders)
	require.Nil(t, err)

	dp, err := consensus.DecodePath("/tree/data/" + path)
	require.Nil(t, err)
	resp, remain, err := testTree.Dag.Resolve(ctx, dp)
	require.Nil(t, err)
	require.Len(t, remain, 0)
	require.Equal(t, value, resp)

	dp, err = consensus.DecodePath("/tree/data/some/data")
	require.Nil(t, err)
	resp, remain, err = testTree.Dag.Resolve(ctx, dp)
	require.Nil(t, err)
	require.Len(t, remain, 0)
	require.Equal(t, "is now set", resp)
}

func TestSetOwnership(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	treeKey, err := crypto.GenerateKey()
	require.Nil(t, err)

	keyAddr := crypto.PubkeyToAddress(treeKey.PublicKey).String()
	keyAddrs := []string{keyAddr}
	treeDID := consensus.AddrToDid(keyAddr)
	nodeStore := nodestore.MustMemoryStore(context.TODO())
	emptyTree := consensus.NewEmptyTree(ctx, treeDID, nodeStore)
	path := "some/data"
	value := "is now set"

	txn, err := chaintree.NewSetDataTransaction(path, value)
	assert.Nil(t, err)

	unsignedBlock := chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       0,
			Transactions: []*transactions.Transaction{txn},
		},
	}
	testTree, err := chaintree.NewChainTree(context.TODO(), emptyTree, nil, consensus.DefaultTransactors)
	require.Nil(t, err)

	blockWithHeaders, err := consensus.SignBlock(ctx, &unsignedBlock, treeKey)
	require.Nil(t, err)

	_, err = testTree.ProcessBlock(context.TODO(), blockWithHeaders)
	require.Nil(t, err)

	dp, err := consensus.DecodePath("/tree/data/" + path)
	require.Nil(t, err)
	resp, remain, err := testTree.Dag.Resolve(context.TODO(), dp)
	require.Nil(t, err)
	require.Len(t, remain, 0)
	require.Equal(t, value, resp)

	txn, err = chaintree.NewSetOwnershipTransaction(keyAddrs)
	assert.Nil(t, err)

	unsignedBlock = chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &testTree.Dag.Tip,
			Height:       1,
			Transactions: []*transactions.Transaction{txn},
		},
	}
	blockWithHeaders, err = consensus.SignBlock(ctx, &unsignedBlock, treeKey)
	require.Nil(t, err)

	_, err = testTree.ProcessBlock(ctx, blockWithHeaders)
	require.Nil(t, err)

	resp, remain, err = testTree.Dag.Resolve(ctx, dp)
	require.Nil(t, err)
	assert.Len(t, remain, 0)
	assert.Equal(t, value, resp)
}

func TestSendToken(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key, err := crypto.GenerateKey()
	assert.Nil(t, err)

	store := nodestore.MustMemoryStore(ctx)
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(key.PublicKey).String())
	emptyTree := consensus.NewEmptyTree(ctx, treeDID, store)

	tokenName := "testtoken"
	tokenFullName := strings.Join([]string{treeDID, tokenName}, ":")

	maximumAmount := uint64(50)
	height := uint64(0)

	establishTxn, err := chaintree.NewEstablishTokenTransaction(tokenName, 0)
	assert.Nil(t, err)

	mintTxn, err := chaintree.NewMintTokenTransaction(tokenName, maximumAmount)
	assert.Nil(t, err)

	blockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       height,
			Transactions: []*transactions.Transaction{establishTxn, mintTxn},
		},
	}

	testTree, err := chaintree.NewChainTree(ctx, emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	_, err = testTree.ProcessBlock(ctx, blockWithHeaders)
	assert.Nil(t, err)
	height++

	targetKey, err := crypto.GenerateKey()
	assert.Nil(t, err)
	targetTreeDID := consensus.AddrToDid(crypto.PubkeyToAddress(targetKey.PublicKey).String())

	txn, err := chaintree.NewSendTokenTransaction("1234", tokenName, 30, targetTreeDID)
	assert.Nil(t, err)

	sendBlockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &testTree.Dag.Tip,
			Height:       height,
			Transactions: []*transactions.Transaction{txn},
		},
	}

	_, err = testTree.ProcessBlock(ctx, sendBlockWithHeaders)
	assert.Nil(t, err)
	height++

	token := consensus.Token{}
	err = testTree.Dag.ResolveInto(ctx, []string{"tree", "_tupelo", "tokens", tokenFullName}, &token)
	assert.Nil(t, err)

	sendsArrayNode, err := testTree.Dag.Get(ctx, *token.Sends)
	assert.Nil(t, err)

	sendNodeCid := sendsArrayNode.Links()[0].Cid
	sendNode, err := testTree.Dag.Get(ctx, sendNodeCid)
	assert.Nil(t, err)

	send := consensus.TokenSend{}
	err = cbornode.DecodeInto(sendNode.RawData(), &send)
	assert.Nil(t, err)

	assert.Equal(t, "1234", send.Id)
	assert.Equal(t, uint64(30), send.Amount)
	assert.Equal(t, targetTreeDID, send.Destination)

	overSpendTxn, err := chaintree.NewSendTokenTransaction("1234", tokenName, (maximumAmount-send.Amount)+1, targetTreeDID)
	assert.Nil(t, err)

	overSpendBlockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &testTree.Dag.Tip,
			Height:       height,
			Transactions: []*transactions.Transaction{overSpendTxn},
		},
	}

	_, err = testTree.ProcessBlock(ctx, overSpendBlockWithHeaders)
	assert.NotNil(t, err)
}

func newTestProof() *gossip.Proof {
	return &gossip.Proof{}
}

func TestReceiveToken(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key, err := crypto.GenerateKey()
	assert.Nil(t, err)

	store := nodestore.MustMemoryStore(context.TODO())
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(key.PublicKey).String())
	emptyTree := consensus.NewEmptyTree(ctx, treeDID, store)

	tokenName1 := "testtoken1"

	maximumAmount := uint64(50)
	senderHeight := uint64(0)
	establishTxn, err := chaintree.NewEstablishTokenTransaction(tokenName1, 0)
	assert.Nil(t, err)

	mintTxn, err := chaintree.NewMintTokenTransaction(tokenName1, maximumAmount)
	assert.Nil(t, err)

	blockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       senderHeight,
			Transactions: []*transactions.Transaction{establishTxn, mintTxn},
		},
	}

	senderChainTree, err := chaintree.NewChainTree(context.TODO(), emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	_, err = senderChainTree.ProcessBlock(context.TODO(), blockWithHeaders)
	assert.Nil(t, err)
	senderHeight++

	// establish & mint another token to ensure we aren't relying on there only being one

	tokenName2 := "testtoken2"
	tokenFullName2 := strings.Join([]string{treeDID, tokenName2}, ":")

	establishTxn2, err := chaintree.NewEstablishTokenTransaction(tokenName2, 0)
	assert.Nil(t, err)

	mintTxn2, err := chaintree.NewMintTokenTransaction(tokenName2, maximumAmount/2)
	assert.Nil(t, err)

	blockWithHeaders = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &senderChainTree.Dag.Tip,
			Height:       senderHeight,
			Transactions: []*transactions.Transaction{establishTxn2, mintTxn2},
		},
	}

	_, err = senderChainTree.ProcessBlock(context.TODO(), blockWithHeaders)
	assert.Nil(t, err)
	senderHeight++

	recipientKey, err := crypto.GenerateKey()
	require.Nil(t, err)
	recipientTreeDID := consensus.AddrToDid(crypto.PubkeyToAddress(recipientKey.PublicKey).String())

	sendTxn, err := chaintree.NewSendTokenTransaction("1234", tokenName2, 20, recipientTreeDID)
	assert.Nil(t, err)

	sendBlockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &senderChainTree.Dag.Tip,
			Height:       senderHeight,
			Transactions: []*transactions.Transaction{sendTxn},
		},
	}

	_, err = senderChainTree.ProcessBlock(context.TODO(), sendBlockWithHeaders)
	assert.Nil(t, err)

	recipientHeight := uint64(0)

	signedBlock, err := consensus.SignBlock(ctx, sendBlockWithHeaders, key)
	require.Nil(t, err)

	headers := &consensus.StandardHeaders{}
	err = typecaster.ToType(signedBlock.Headers, headers)
	require.Nil(t, err)

	tokenPath := []string{"tree", "_tupelo", "tokens", tokenFullName2, consensus.TokenSendLabel, "0"}
	leafNodes, err := senderChainTree.Dag.NodesForPath(context.TODO(), tokenPath)
	require.Nil(t, err)

	leaves := make([][]byte, 0)
	for _, ln := range leafNodes {
		leaves = append(leaves, ln.RawData())
	}

	prf := newTestProof()
	receiveTxn, err := chaintree.NewReceiveTokenTransaction("1234", senderChainTree.Dag.Tip.Bytes(), prf, leaves)
	assert.Nil(t, err)

	receiveBlockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       recipientHeight,
			Transactions: []*transactions.Transaction{receiveTxn},
		},
	}

	recipientChainTree, err := consensus.NewSignedChainTree(ctx, recipientKey.PublicKey, store)
	require.Nil(t, err)
	recipientChainTree.ChainTree.BlockValidators = append(recipientChainTree.ChainTree.BlockValidators, types.IsTokenRecipient)

	valid, err := recipientChainTree.ChainTree.ProcessBlock(context.TODO(), receiveBlockWithHeaders)
	assert.Nil(t, err)
	assert.True(t, valid)

	senderTree, err := senderChainTree.Tree(context.TODO())
	require.Nil(t, err)

	tokenCanonicalName2, err := consensus.CanonicalTokenName(senderTree, treeDID, tokenName2, true)
	require.Nil(t, err)

	senderLedger := consensus.NewTreeLedger(senderTree, tokenCanonicalName2)
	senderBalance, err := senderLedger.Balance()
	require.Nil(t, err)

	recipientTree, err := recipientChainTree.ChainTree.Tree(context.TODO())
	require.Nil(t, err)
	recipientLedger := consensus.NewTreeLedger(recipientTree, tokenCanonicalName2)
	recipientBalance, err := recipientLedger.Balance()
	require.Nil(t, err)

	assert.Equal(t, uint64(5), senderBalance)
	assert.Equal(t, uint64(20), recipientBalance)
}

func TestReceiveTokenInvalidTip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key, err := crypto.GenerateKey()
	assert.Nil(t, err)

	store := nodestore.MustMemoryStore(context.TODO())
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(key.PublicKey).String())
	emptyTree := consensus.NewEmptyTree(ctx, treeDID, store)

	tokenName1 := "testtoken1"

	maximumAmount := uint64(50)
	senderHeight := uint64(0)
	establishTxn, err := chaintree.NewEstablishTokenTransaction(tokenName1, 0)
	assert.Nil(t, err)

	mintTxn, err := chaintree.NewMintTokenTransaction(tokenName1, maximumAmount)
	assert.Nil(t, err)

	blockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       senderHeight,
			Transactions: []*transactions.Transaction{establishTxn, mintTxn},
		},
	}

	senderChainTree, err := chaintree.NewChainTree(context.TODO(), emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	_, err = senderChainTree.ProcessBlock(context.TODO(), blockWithHeaders)
	assert.Nil(t, err)
	senderHeight++

	// establish & mint another token to ensure we aren't relying on there only being one

	tokenName2 := "testtoken2"
	tokenFullName2 := strings.Join([]string{treeDID, tokenName2}, ":")

	establishTxn2, err := chaintree.NewEstablishTokenTransaction(tokenName2, 0)
	assert.Nil(t, err)

	mintTxn2, err := chaintree.NewMintTokenTransaction(tokenName2, maximumAmount/2)
	assert.Nil(t, err)

	blockWithHeaders = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &senderChainTree.Dag.Tip,
			Height:       senderHeight,
			Transactions: []*transactions.Transaction{establishTxn2, mintTxn2},
		},
	}

	_, err = senderChainTree.ProcessBlock(context.TODO(), blockWithHeaders)
	assert.Nil(t, err)
	senderHeight++

	recipientKey, err := crypto.GenerateKey()
	require.Nil(t, err)
	recipientTreeDID := consensus.AddrToDid(crypto.PubkeyToAddress(recipientKey.PublicKey).String())

	sendTxn, err := chaintree.NewSendTokenTransaction("1234", tokenName2, 20, recipientTreeDID)
	assert.Nil(t, err)

	sendBlockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &senderChainTree.Dag.Tip,
			Height:       senderHeight,
			Transactions: []*transactions.Transaction{sendTxn},
		},
	}

	_, err = senderChainTree.ProcessBlock(context.TODO(), sendBlockWithHeaders)
	assert.Nil(t, err)

	recipientHeight := uint64(0)

	signedBlock, err := consensus.SignBlock(ctx, sendBlockWithHeaders, key)
	require.Nil(t, err)

	headers := &consensus.StandardHeaders{}
	err = typecaster.ToType(signedBlock.Headers, headers)
	require.Nil(t, err)

	tokenPath := []string{"tree", "_tupelo", "tokens", tokenFullName2, consensus.TokenSendLabel, "0"}
	leafNodes, err := senderChainTree.Dag.NodesForPath(context.TODO(), tokenPath)
	require.Nil(t, err)

	leaves := make([][]byte, 0)
	for _, ln := range leafNodes {
		leaves = append(leaves, ln.RawData())
	}

	otherChainTree, err := chaintree.NewChainTree(context.TODO(), emptyTree, nil, consensus.DefaultTransactors)
	require.Nil(t, err)

	prf := newTestProof()
	receiveTxn, err := chaintree.NewReceiveTokenTransaction("1234", otherChainTree.Dag.Tip.Bytes(), prf, leaves)
	assert.Nil(t, err)

	receiveBlockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       recipientHeight,
			Transactions: []*transactions.Transaction{receiveTxn},
		},
	}

	recipientChainTree, err := consensus.NewSignedChainTree(ctx, recipientKey.PublicKey, store)
	require.Nil(t, err)
	recipientChainTree.ChainTree.BlockValidators = append(recipientChainTree.ChainTree.BlockValidators, types.IsTokenRecipient)

	valid, err := recipientChainTree.ChainTree.ProcessBlock(context.TODO(), receiveBlockWithHeaders)
	assert.NotNil(t, err)
	assert.False(t, valid)
}

func TestReceiveTokenInvalidDoubleReceive(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key, err := crypto.GenerateKey()
	assert.Nil(t, err)

	store := nodestore.MustMemoryStore(context.TODO())
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(key.PublicKey).String())
	emptyTree := consensus.NewEmptyTree(ctx, treeDID, store)

	tokenName1 := "testtoken1"

	maximumAmount := uint64(50)
	senderHeight := uint64(0)
	establishTxn, err := chaintree.NewEstablishTokenTransaction(tokenName1, 0)
	assert.Nil(t, err)

	mintTxn, err := chaintree.NewMintTokenTransaction(tokenName1, maximumAmount)
	assert.Nil(t, err)

	blockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       senderHeight,
			Transactions: []*transactions.Transaction{establishTxn, mintTxn},
		},
	}

	senderChainTree, err := chaintree.NewChainTree(context.TODO(), emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	_, err = senderChainTree.ProcessBlock(context.TODO(), blockWithHeaders)
	assert.Nil(t, err)
	senderHeight++

	// establish & mint another token to ensure we aren't relying on there only being one

	tokenName2 := "testtoken2"
	tokenFullName2 := strings.Join([]string{treeDID, tokenName2}, ":")

	establishTxn2, err := chaintree.NewEstablishTokenTransaction(tokenName2, 0)
	assert.Nil(t, err)

	mintTxn2, err := chaintree.NewMintTokenTransaction(tokenName2, maximumAmount/2)
	assert.Nil(t, err)

	blockWithHeaders = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &senderChainTree.Dag.Tip,
			Height:       senderHeight,
			Transactions: []*transactions.Transaction{establishTxn2, mintTxn2},
		},
	}

	_, err = senderChainTree.ProcessBlock(context.TODO(), blockWithHeaders)
	assert.Nil(t, err)
	senderHeight++

	recipientKey, err := crypto.GenerateKey()
	require.Nil(t, err)
	recipientTreeDID := consensus.AddrToDid(crypto.PubkeyToAddress(recipientKey.PublicKey).String())

	sendTxn, err := chaintree.NewSendTokenTransaction("1234", tokenName2, 20, recipientTreeDID)
	assert.Nil(t, err)

	sendBlockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &senderChainTree.Dag.Tip,
			Height:       senderHeight,
			Transactions: []*transactions.Transaction{sendTxn},
		},
	}

	_, err = senderChainTree.ProcessBlock(context.TODO(), sendBlockWithHeaders)
	assert.Nil(t, err)
	senderHeight++

	recipientHeight := uint64(0)

	signedBlock, err := consensus.SignBlock(ctx, sendBlockWithHeaders, key)
	require.Nil(t, err)

	headers := &consensus.StandardHeaders{}
	err = typecaster.ToType(signedBlock.Headers, headers)
	require.Nil(t, err)

	tokenPath := []string{"tree", "_tupelo", "tokens", tokenFullName2, consensus.TokenSendLabel, "0"}
	leafNodes, err := senderChainTree.Dag.NodesForPath(context.TODO(), tokenPath)
	require.Nil(t, err)

	leaves := make([][]byte, 0)
	for _, ln := range leafNodes {
		leaves = append(leaves, ln.RawData())
	}

	prf := newTestProof()
	receiveTxn, err := chaintree.NewReceiveTokenTransaction("1234", senderChainTree.Dag.Tip.Bytes(), prf, leaves)
	assert.Nil(t, err)

	receiveBlockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       recipientHeight,
			Transactions: []*transactions.Transaction{receiveTxn},
		},
	}

	recipientChainTree, err := consensus.NewSignedChainTree(ctx, recipientKey.PublicKey, store)
	require.Nil(t, err)
	recipientChainTree.ChainTree.BlockValidators = append(recipientChainTree.ChainTree.BlockValidators, types.IsTokenRecipient)

	valid, err := recipientChainTree.ChainTree.ProcessBlock(context.TODO(), receiveBlockWithHeaders)
	assert.Nil(t, err)
	assert.True(t, valid)
	recipientHeight++

	// now attempt to receive a new, otherwise valid send w/ the same transaction id (which should fail)

	sendTxn2, err := chaintree.NewSendTokenTransaction("1234", tokenName2, 2, recipientTreeDID)
	assert.Nil(t, err)

	sendBlockWithHeaders = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &senderChainTree.Dag.Tip,
			Height:       senderHeight,
			Transactions: []*transactions.Transaction{sendTxn2},
		},
	}

	_, err = senderChainTree.ProcessBlock(context.TODO(), sendBlockWithHeaders)
	assert.Nil(t, err)

	signedBlock, err = consensus.SignBlock(ctx, sendBlockWithHeaders, key)
	require.Nil(t, err)

	headers = &consensus.StandardHeaders{}
	err = typecaster.ToType(signedBlock.Headers, headers)
	require.Nil(t, err)

	tokenPath = []string{"tree", "_tupelo", "tokens", tokenFullName2, consensus.TokenSendLabel, "1"}
	leafNodes, err = senderChainTree.Dag.NodesForPath(context.TODO(), tokenPath)
	require.Nil(t, err)

	leaves = make([][]byte, 0)
	for _, ln := range leafNodes {
		leaves = append(leaves, ln.RawData())
	}

	receiveTxn, err = chaintree.NewReceiveTokenTransaction("1234", senderChainTree.Dag.Tip.Bytes(), prf, leaves)
	assert.Nil(t, err)

	receiveBlockWithHeaders = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &recipientChainTree.ChainTree.Dag.Tip,
			Height:       recipientHeight,
			Transactions: []*transactions.Transaction{receiveTxn},
		},
	}

	valid, err = recipientChainTree.ChainTree.ProcessBlock(context.TODO(), receiveBlockWithHeaders)
	assert.NotNil(t, err)
	assert.False(t, valid)
}

func TestReceiveTokenInvalidSignature(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key, err := crypto.GenerateKey()
	assert.Nil(t, err)

	store := nodestore.MustMemoryStore(context.TODO())
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(key.PublicKey).String())
	emptyTree := consensus.NewEmptyTree(ctx, treeDID, store)

	tokenName1 := "testtoken1"

	maximumAmount := uint64(50)
	senderHeight := uint64(0)
	establishTxn, err := chaintree.NewEstablishTokenTransaction(tokenName1, 0)
	assert.Nil(t, err)

	mintTxn, err := chaintree.NewMintTokenTransaction(tokenName1, maximumAmount)
	assert.Nil(t, err)

	blockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       senderHeight,
			Transactions: []*transactions.Transaction{establishTxn, mintTxn},
		},
	}

	senderChainTree, err := chaintree.NewChainTree(context.TODO(), emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	_, err = senderChainTree.ProcessBlock(context.TODO(), blockWithHeaders)
	assert.Nil(t, err)
	senderHeight++

	// establish & mint another token to ensure we aren't relying on there only being one

	tokenName2 := "testtoken2"
	tokenFullName2 := strings.Join([]string{treeDID, tokenName2}, ":")

	establishTxn2, err := chaintree.NewEstablishTokenTransaction(tokenName2, 0)
	assert.Nil(t, err)

	mintTxn2, err := chaintree.NewMintTokenTransaction(tokenName2, maximumAmount/2)
	assert.Nil(t, err)

	blockWithHeaders = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &senderChainTree.Dag.Tip,
			Height:       senderHeight,
			Transactions: []*transactions.Transaction{establishTxn2, mintTxn2},
		},
	}

	_, err = senderChainTree.ProcessBlock(context.TODO(), blockWithHeaders)
	assert.Nil(t, err)
	senderHeight++

	recipientKey, err := crypto.GenerateKey()
	require.Nil(t, err)
	recipientTreeDID := consensus.AddrToDid(crypto.PubkeyToAddress(recipientKey.PublicKey).String())

	sendTxn, err := chaintree.NewSendTokenTransaction("1234", tokenName2, 20, recipientTreeDID)
	assert.Nil(t, err)

	sendBlockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &senderChainTree.Dag.Tip,
			Height:       senderHeight,
			Transactions: []*transactions.Transaction{sendTxn},
		},
	}

	_, err = senderChainTree.ProcessBlock(context.TODO(), sendBlockWithHeaders)
	assert.Nil(t, err)

	otherKey, err := crypto.GenerateKey()
	require.Nil(t, err)

	signedBlock, err := consensus.SignBlock(ctx, sendBlockWithHeaders, otherKey)
	require.Nil(t, err)

	headers := &consensus.StandardHeaders{}
	err = typecaster.ToType(signedBlock.Headers, headers)
	require.Nil(t, err)

	objectID, err := senderChainTree.Id(context.TODO())
	require.Nil(t, err)

	tokenPath := []string{"tree", "_tupelo", "tokens", tokenFullName2, consensus.TokenSendLabel, "0"}
	leafNodes, err := senderChainTree.Dag.NodesForPath(context.TODO(), tokenPath)
	require.Nil(t, err)

	leaves := make([][]byte, 0)
	for _, ln := range leafNodes {
		leaves = append(leaves, ln.RawData())
	}

	recipientHeight := uint64(0)

	prf := &gossip.Proof{
		ObjectId: []byte(objectID),
		Tip:      senderChainTree.Dag.Tip.Bytes(),
	}

	receiveTxn, err := chaintree.NewReceiveTokenTransaction("1234", senderChainTree.Dag.Tip.Bytes(), prf, leaves)
	assert.Nil(t, err)

	receiveBlockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       recipientHeight,
			Transactions: []*transactions.Transaction{receiveTxn},
		},
	}

	recipientChainTree, err := consensus.NewSignedChainTree(ctx, recipientKey.PublicKey, store)
	require.Nil(t, err)

	hasValidProof := types.GenerateHasValidProof(func(proof *gossip.Proof) (bool, error) {
		return false, nil
	})

	recipientChainTree.ChainTree.BlockValidators = append(recipientChainTree.ChainTree.BlockValidators,
		types.IsTokenRecipient, hasValidProof)

	valid, err := recipientChainTree.ChainTree.ProcessBlock(context.TODO(), receiveBlockWithHeaders)
	assert.Nil(t, err)
	assert.False(t, valid)
}

func TestReceiveTokenInvalidDestinationChainId(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key, err := crypto.GenerateKey()
	assert.Nil(t, err)

	store := nodestore.MustMemoryStore(context.TODO())
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(key.PublicKey).String())
	emptyTree := consensus.NewEmptyTree(ctx, treeDID, store)

	tokenName1 := "testtoken1"

	maximumAmount := uint64(50)
	senderHeight := uint64(0)

	establishTxn, err := chaintree.NewEstablishTokenTransaction(tokenName1, 0)
	assert.Nil(t, err)

	mintTxn, err := chaintree.NewMintTokenTransaction(tokenName1, maximumAmount)
	assert.Nil(t, err)

	blockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       senderHeight,
			Transactions: []*transactions.Transaction{establishTxn, mintTxn},
		},
	}

	senderChainTree, err := chaintree.NewChainTree(context.TODO(), emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	_, err = senderChainTree.ProcessBlock(context.TODO(), blockWithHeaders)
	assert.Nil(t, err)
	senderHeight++

	// establish & mint another token to ensure we aren't relying on there only being one

	tokenName2 := "testtoken2"
	tokenFullName2 := strings.Join([]string{treeDID, tokenName2}, ":")

	establishTxn2, err := chaintree.NewEstablishTokenTransaction(tokenName2, 0)
	assert.Nil(t, err)

	mintTxn2, err := chaintree.NewMintTokenTransaction(tokenName2, maximumAmount/2)
	assert.Nil(t, err)

	blockWithHeaders = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &senderChainTree.Dag.Tip,
			Height:       senderHeight,
			Transactions: []*transactions.Transaction{establishTxn2, mintTxn2},
		},
	}

	_, err = senderChainTree.ProcessBlock(context.TODO(), blockWithHeaders)
	assert.Nil(t, err)
	senderHeight++

	otherKey, err := crypto.GenerateKey()
	require.Nil(t, err)
	otherTreeDID := consensus.AddrToDid(crypto.PubkeyToAddress(otherKey.PublicKey).String())

	sendTxn, err := chaintree.NewSendTokenTransaction("1234", tokenName2, 20, otherTreeDID)
	assert.Nil(t, err)

	sendBlockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &senderChainTree.Dag.Tip,
			Height:       senderHeight,
			Transactions: []*transactions.Transaction{sendTxn},
		},
	}

	_, err = senderChainTree.ProcessBlock(context.TODO(), sendBlockWithHeaders)
	assert.Nil(t, err)

	signedBlock, err := consensus.SignBlock(ctx, sendBlockWithHeaders, key)
	require.Nil(t, err)

	headers := &consensus.StandardHeaders{}
	err = typecaster.ToType(signedBlock.Headers, headers)
	require.Nil(t, err)

	tokenPath := []string{"tree", "_tupelo", "tokens", tokenFullName2, consensus.TokenSendLabel, "0"}
	leafNodes, err := senderChainTree.Dag.NodesForPath(context.TODO(), tokenPath)
	require.Nil(t, err)

	leaves := make([][]byte, 0)
	for _, ln := range leafNodes {
		leaves = append(leaves, ln.RawData())
	}

	recipientHeight := uint64(0)

	prf := newTestProof()
	receiveTxn, err := chaintree.NewReceiveTokenTransaction("1234", senderChainTree.Dag.Tip.Bytes(), prf, leaves)
	assert.Nil(t, err)

	receiveBlockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       recipientHeight,
			Transactions: []*transactions.Transaction{receiveTxn},
		},
	}

	recipientKey, err := crypto.GenerateKey()
	require.Nil(t, err)
	recipientChainTree, err := consensus.NewSignedChainTree(ctx, recipientKey.PublicKey, store)
	require.Nil(t, err)
	recipientChainTree.ChainTree.BlockValidators = append(recipientChainTree.ChainTree.BlockValidators, types.IsTokenRecipient)

	valid, err := recipientChainTree.ChainTree.ProcessBlock(context.TODO(), receiveBlockWithHeaders)
	assert.Nil(t, err)
	assert.False(t, valid)
}

func TestReceiveTokenMismatchedSignatureTip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key, err := crypto.GenerateKey()
	assert.Nil(t, err)

	store := nodestore.MustMemoryStore(context.TODO())
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(key.PublicKey).String())
	emptyTree := consensus.NewEmptyTree(ctx, treeDID, store)

	tokenName1 := "testtoken1"

	maximumAmount := uint64(50)
	senderHeight := uint64(0)
	establishTxn, err := chaintree.NewEstablishTokenTransaction(tokenName1, 0)
	assert.Nil(t, err)

	mintTxn, err := chaintree.NewMintTokenTransaction(tokenName1, maximumAmount)
	assert.Nil(t, err)

	blockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       senderHeight,
			Transactions: []*transactions.Transaction{establishTxn, mintTxn},
		},
	}

	senderChainTree, err := chaintree.NewChainTree(context.TODO(), emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	_, err = senderChainTree.ProcessBlock(context.TODO(), blockWithHeaders)
	assert.Nil(t, err)
	senderHeight++

	// establish & mint another token to ensure we aren't relying on there only being one

	tokenName2 := "testtoken2"
	tokenFullName2 := strings.Join([]string{treeDID, tokenName2}, ":")

	establishTxn2, err := chaintree.NewEstablishTokenTransaction(tokenName2, 0)
	assert.Nil(t, err)

	mintTxn2, err := chaintree.NewMintTokenTransaction(tokenName2, maximumAmount/2)
	assert.Nil(t, err)

	blockWithHeaders = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &senderChainTree.Dag.Tip,
			Height:       senderHeight,
			Transactions: []*transactions.Transaction{establishTxn2, mintTxn2},
		},
	}

	_, err = senderChainTree.ProcessBlock(context.TODO(), blockWithHeaders)
	assert.Nil(t, err)
	senderHeight++

	recipientKey, err := crypto.GenerateKey()
	require.Nil(t, err)
	recipientTreeDID := consensus.AddrToDid(crypto.PubkeyToAddress(recipientKey.PublicKey).String())

	sendTxn, err := chaintree.NewSendTokenTransaction("1234", tokenName2, 20, recipientTreeDID)
	assert.Nil(t, err)

	sendBlockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  &senderChainTree.Dag.Tip,
			Height:       senderHeight,
			Transactions: []*transactions.Transaction{sendTxn},
		},
	}

	_, err = senderChainTree.ProcessBlock(context.TODO(), sendBlockWithHeaders)
	assert.Nil(t, err)

	otherKey, err := crypto.GenerateKey()
	require.Nil(t, err)

	signedBlock, err := consensus.SignBlock(ctx, sendBlockWithHeaders, otherKey)
	require.Nil(t, err)

	headers := &consensus.StandardHeaders{}
	err = typecaster.ToType(signedBlock.Headers, headers)
	require.Nil(t, err)

	objectID, err := senderChainTree.Id(context.TODO())
	require.Nil(t, err)

	tokenPath := []string{"tree", "_tupelo", "tokens", tokenFullName2, consensus.TokenSendLabel, "0"}
	leafNodes, err := senderChainTree.Dag.NodesForPath(context.TODO(), tokenPath)
	require.Nil(t, err)

	leaves := make([][]byte, 0)
	for _, ln := range leafNodes {
		leaves = append(leaves, ln.RawData())
	}

	recipientHeight := uint64(0)

	prf := &gossip.Proof{
		ObjectId: []byte(objectID),
		Tip:      emptyTree.Tip.Bytes(), // invalid

	}
	receiveTxn, err := chaintree.NewReceiveTokenTransaction("1234", senderChainTree.Dag.Tip.Bytes(), prf, leaves)
	assert.Nil(t, err)

	receiveBlockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       recipientHeight,
			Transactions: []*transactions.Transaction{receiveTxn},
		},
	}

	recipientChainTree, err := consensus.NewSignedChainTree(ctx, recipientKey.PublicKey, store)
	require.Nil(t, err)

	hasValidProof := types.GenerateHasValidProof(func(proof *gossip.Proof) (bool, error) {
		return true, nil // this should get caught before it gets here; so ensure this doesn't cause false positives
	})

	recipientChainTree.ChainTree.BlockValidators = append(recipientChainTree.ChainTree.BlockValidators,
		types.IsTokenRecipient, hasValidProof)

	valid, err := recipientChainTree.ChainTree.ProcessBlock(context.TODO(), receiveBlockWithHeaders)
	assert.Nil(t, err)
	assert.False(t, valid)
}

func TestDecodePath(t *testing.T) {
	dp1, err := consensus.DecodePath("/some/data")
	assert.Nil(t, err)
	assert.Equal(t, []string{"some", "data"}, dp1)

	dp2, err := consensus.DecodePath("some/data")
	assert.Nil(t, err)
	assert.Equal(t, []string{"some", "data"}, dp2)

	dp3, err := consensus.DecodePath("//some/data")
	assert.NotNil(t, err)
	assert.Nil(t, dp3)

	dp4, err := consensus.DecodePath("/some//data")
	assert.NotNil(t, err)
	assert.Nil(t, dp4)

	dp5, err := consensus.DecodePath("/some/../data")
	assert.Nil(t, err)
	assert.Equal(t, []string{"some", "..", "data"}, dp5)

	dp6, err := consensus.DecodePath("")
	assert.Nil(t, err)
	assert.Equal(t, []string{}, dp6)

	dp7, err := consensus.DecodePath("/")
	assert.Nil(t, err)
	assert.Equal(t, []string{}, dp7)

	dp8, err := consensus.DecodePath("/_tupelo")
	assert.Nil(t, err)
	assert.Equal(t, []string{"_tupelo"}, dp8)

	dp9, err := consensus.DecodePath("_tupelo")
	assert.Nil(t, err)
	assert.Equal(t, []string{"_tupelo"}, dp9)

	dp10, err := consensus.DecodePath("//_tupelo")
	assert.NotNil(t, err)
	assert.Nil(t, dp10)
}
