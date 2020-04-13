package testhelpers

import (
	"context"
	"crypto/ecdsa"
	"testing"

	"github.com/quorumcontrol/messages/v2/build/go/services"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/transactions"
	"github.com/stretchr/testify/require"

	"github.com/quorumcontrol/tupelo/sdk/consensus"
)

func NewValidTransaction(t testing.TB) services.AddBlockRequest {
	treeKey, err := crypto.GenerateKey()
	require.Nil(t, err)

	return NewValidTransactionWithKey(t, treeKey)
}

func NewValidTransactionWithKey(t testing.TB, treeKey *ecdsa.PrivateKey) services.AddBlockRequest {
	return NewValidTransactionWithPathAndValue(t, treeKey, "down/in/the/thing", "hi")
}

func NewValidTransactionWithPathAndValue(t testing.TB, treeKey *ecdsa.PrivateKey, path, value string) services.AddBlockRequest {
	ctx := context.TODO()
	sw := safewrap.SafeWrap{}

	txn, err := chaintree.NewSetDataTransaction(path, value)
	require.Nil(t, err)

	unsignedBlock := chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       0,
			Transactions: []*transactions.Transaction{txn},
		},
	}

	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())

	nodeStore := nodestore.MustMemoryStore(ctx)
	emptyTree := consensus.NewEmptyTree(ctx, treeDID, nodeStore)
	emptyTip := emptyTree.Tip
	testTree, err := chaintree.NewChainTree(ctx, emptyTree, nil, consensus.DefaultTransactors)
	require.Nil(t, err)

	blockWithHeaders, err := consensus.SignBlock(ctx, &unsignedBlock, treeKey)
	require.Nil(t, err)

	_, err = testTree.ProcessBlock(ctx, blockWithHeaders)
	require.Nil(t, err)
	nodes := DagToByteNodes(t, testTree.Dag)

	bits := sw.WrapObject(blockWithHeaders).RawData()
	require.Nil(t, sw.Err)

	return services.AddBlockRequest{
		PreviousTip: emptyTip.Bytes(),
		Height:      blockWithHeaders.Height,
		NewTip:      testTree.Dag.Tip.Bytes(),
		Payload:     bits,
		State:       nodes,
		ObjectId:    []byte(treeDID),
	}
}

func DagToByteNodes(t testing.TB, dagTree *dag.Dag) [][]byte {
	cborNodes, err := dagTree.Nodes(context.TODO())
	require.Nil(t, err)
	nodes := make([][]byte, len(cborNodes))
	for i, node := range cborNodes {
		nodes[i] = node.RawData()
	}
	return nodes
}
