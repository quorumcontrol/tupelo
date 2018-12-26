package actors

import (
	"crypto/ecdsa"
	"testing"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/stretchr/testify/require"
)

func newValidTransaction(t testing.TB) messages.Transaction {
	treeKey, err := crypto.GenerateKey()
	require.Nil(t, err)

	return newValidTransactionWithKey(t, treeKey)
}

func newValidTransactionWithKey(t testing.TB, treeKey *ecdsa.PrivateKey) messages.Transaction {
	return newValidTransactionWithPathAndValue(t, treeKey, "down/in/the/thing", "hi")
}

func newValidTransactionWithPathAndValue(t testing.TB, treeKey *ecdsa.PrivateKey, path, value string) messages.Transaction {
	sw := safewrap.SafeWrap{}

	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]string{
						"path":  path,
						"value": value,
					},
				},
			},
		},
	}

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	emptyTree := consensus.NewEmptyTree(treeDID, nodeStore)
	emptyTip := emptyTree.Tip
	testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)
	require.Nil(t, err)

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	require.Nil(t, err)

	testTree.ProcessBlock(blockWithHeaders)
	nodes := dagToByteNodes(t, emptyTree)

	req := &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      &emptyTree.Tip,
		NewBlock: blockWithHeaders,
	}
	return messages.Transaction{
		PreviousTip: emptyTip.Bytes(),
		NewTip:      testTree.Dag.Tip.Bytes(),
		Payload:     sw.WrapObject(req).RawData(),
		ObjectID:    []byte(treeDID),
	}
}

func dagToByteNodes(t testing.TB, dagTree *dag.Dag) [][]byte {
	cborNodes, err := dagTree.Nodes()
	require.Nil(t, err)
	nodes := make([][]byte, len(cborNodes))
	for i, node := range cborNodes {
		nodes[i] = node.RawData()
	}
	return nodes
}

var nullActorFunc = func(_ actor.Context) {}

func newNullActorProps() *actor.Props {
	return actor.FromFunc(nullActorFunc)
}