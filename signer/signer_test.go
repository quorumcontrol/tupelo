package signer

import (
	"testing"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/stretchr/testify/assert"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestSigner_ProcessRequest(t *testing.T) {
	key,err := bls.NewSignKey()
	assert.Nil(t, err)

	pubKey := consensus.BlsKeyToPublicKey(key.MustVerKey())
	group := &Group{
		SortedPublicKeys: []*consensus.PublicKey{&pubKey},
	}

	store := storage.NewMemStorage()
	store.CreateBucketIfNotExists(DidBucket)

	signer := &Signer{
		Storage: store,
		Group: group,
		Id: consensus.BlsVerKeyToAddress(key.MustVerKey().Bytes()).String(),
		SignKey: key,
		VerKey: key.MustVerKey(),
	}

	treeKey,err := crypto.GenerateKey()
	assert.Nil(t,err)

	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]string{
						"path": "down/in/the/thing",
						"value": "hi",
					},
				},
			},
		},
	}

	emptyTree := consensus.NewEmptyTree(treeDID)

	nodes := make([][]byte, len(emptyTree.Nodes()))
	for i,node := range emptyTree.Nodes() {
		nodes[i] = node.Node.RawData()
	}

	blockWithHeaders,err := consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	req := &AddBlockRequest{
		Nodes: nodes,
		Tip: emptyTree.Tip,
		NewBlock: blockWithHeaders,
	}

	resp,err := signer.ProcessRequest(req)

	assert.Nil(t, err)

	testTree,err := chaintree.NewChainTree(emptyTree, nil, transactors)
	assert.Nil(t, err)

	testTree.ProcessBlock(blockWithHeaders)

	assert.Equal(t, resp.Tip, testTree.Dag.Tip)

}
