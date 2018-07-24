package signer

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/stretchr/testify/assert"
)

func createSigner(t *testing.T) *Signer {
	key, err := bls.NewSignKey()
	assert.Nil(t, err)

	pubKey := consensus.BlsKeyToPublicKey(key.MustVerKey())

	dstKey, err := crypto.GenerateKey()
	assert.Nil(t, err)
	dstPubKey := consensus.EcdsaToPublicKey(&dstKey.PublicKey)

	group := consensus.NewGroup([]*consensus.RemoteNode{consensus.NewRemoteNode(pubKey, dstPubKey)})

	signer := &Signer{
		Group:   group,
		Id:      consensus.BlsVerKeyToAddress(key.MustVerKey().Bytes()).String(),
		SignKey: key,
		VerKey:  key.MustVerKey(),
	}

	return signer
}

func TestSigner_ProcessRequest(t *testing.T) {

	signer := createSigner(t)

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

	emptyTree := consensus.NewEmptyTree(treeDID)

	nodes := make([][]byte, len(emptyTree.Nodes()))
	for i, node := range emptyTree.Nodes() {
		nodes[i] = node.Node.RawData()
	}

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	req := &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      emptyTree.Tip,
		NewBlock: blockWithHeaders,
	}

	resp, err := signer.ProcessAddBlock(nil, req)

	assert.Nil(t, err)

	testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	testTree.ProcessBlock(blockWithHeaders)
	assert.Equal(t, resp.Tip, testTree.Dag.Tip)

	resp, err = signer.ProcessAddBlock(resp.Tip, req)
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

	nodes = make([][]byte, len(testTree.Dag.Nodes()))
	for i, node := range testTree.Dag.Nodes() {
		nodes[i] = node.Node.RawData()
	}

	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	req = &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      testTree.Dag.Tip,
		NewBlock: blockWithHeaders,
	}

	resp, err = signer.ProcessAddBlock(emptyTree.Tip, req)
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

	nodes = make([][]byte, len(testTree.Dag.Nodes()))
	for i, node := range testTree.Dag.Nodes() {
		nodes[i] = node.Node.RawData()
	}

	req = &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      emptyTree.Tip,
		NewBlock: blockWithHeaders,
	}

	resp, err = signer.ProcessAddBlock(emptyTree.Tip, req)
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

	nodes = make([][]byte, len(testTree.Dag.Nodes()))
	for i, node := range testTree.Dag.Nodes() {
		nodes[i] = node.Node.RawData()
	}

	req = &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      emptyTree.Tip,
		NewBlock: blockWithHeaders,
	}

	resp, err = signer.ProcessAddBlock(emptyTree.Tip, req)
	assert.NotNil(t, err)

	// however if we sign it with the new owner, it should be accepted.
	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, newOwnerKey)
	assert.Nil(t, err)

	req = &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      emptyTree.Tip,
		NewBlock: blockWithHeaders,
	}

	resp, err = signer.ProcessAddBlock(emptyTree.Tip, req)
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
						"path":  "_qc/authentications/publicKey",
						"value": "test",
					},
				},
			},
		},
	}

	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, newOwnerKey)
	assert.Nil(t, err)

	nodes = make([][]byte, len(testTree.Dag.Nodes()))
	for i, node := range testTree.Dag.Nodes() {
		nodes[i] = node.Node.RawData()
	}

	req = &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      emptyTree.Tip,
		NewBlock: blockWithHeaders,
	}

	resp, err = signer.ProcessAddBlock(emptyTree.Tip, req)
	assert.NotNil(t, err)
}

func TestSigner_NextBlockValidation(t *testing.T) {
	signer := createSigner(t)

	treeKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())
	emptyTree := consensus.NewEmptyTree(treeDID)
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

	nodes1 := make([][]byte, len(emptyTree.Nodes()))
	for i, node := range emptyTree.Nodes() {
		nodes1[i] = node.Node.RawData()
	}

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	req := &consensus.AddBlockRequest{
		Nodes:    nodes1,
		Tip:      emptyTree.Tip,
		NewBlock: blockWithHeaders,
	}

	resp, err := signer.ProcessAddBlock(emptyTree.Tip, req)
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

	nodes2 := make([][]byte, len(emptyTree.Nodes()))
	for i, node := range emptyTree.Nodes() {
		nodes2[i] = node.Node.RawData()
	}

	blockWithHeaders2, err := consensus.SignBlock(unsignedBlock2, treeKey)
	assert.Nil(t, err)

	req2 := &consensus.AddBlockRequest{
		Nodes:    nodes2,
		Tip:      resp.Tip,
		NewBlock: blockWithHeaders2,
	}

	resp2, err := signer.ProcessAddBlock(emptyTree.Tip, req2)
	assert.Nil(t, err)

	testTree.ProcessBlock(blockWithHeaders2)
	assert.Equal(t, resp2.Tip, testTree.Dag.Tip)

	nodes3 := make([][]byte, len(emptyTree.Nodes()))
	for i, node := range emptyTree.Nodes() {
		nodes3[i] = node.Node.RawData()
	}

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

	_, err = signer.ProcessAddBlock(emptyTree.Tip, req3)
	assert.NotNil(t, err)
}
