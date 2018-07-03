package signer

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/stretchr/testify/assert"
)

func createSigner(t *testing.T) *Signer {
	key, err := bls.NewSignKey()
	assert.Nil(t, err)

	pubKey := consensus.BlsKeyToPublicKey(key.MustVerKey())
	group := &consensus.Group{
		SortedPublicKeys: []consensus.PublicKey{pubKey},
	}

	store := storage.NewMemStorage()

	signer := &Signer{
		Storage: store,
		Group:   group,
		Id:      consensus.BlsVerKeyToAddress(key.MustVerKey().Bytes()).String(),
		SignKey: key,
		VerKey:  key.MustVerKey(),
	}

	signer.SetupStorage()

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

	resp, err := signer.ProcessAddBlock(req)

	assert.Nil(t, err)

	testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	testTree.ProcessBlock(blockWithHeaders)

	assert.Equal(t, resp.Tip, testTree.Dag.Tip)

	// replaying should error

	resp, err = signer.ProcessAddBlock(req)
	assert.NotNil(t, err)

	// playing a new transaction should work when there are no auths

	nodes = make([][]byte, len(testTree.Dag.Nodes()))
	for i, node := range testTree.Dag.Nodes() {
		nodes[i] = node.Node.RawData()
	}

	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	req = &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      emptyTree.Tip,
		NewBlock: blockWithHeaders,
	}

	resp, err = signer.ProcessAddBlock(req)
	assert.Nil(t, err)

	testTree.ProcessBlock(blockWithHeaders)

	assert.Equal(t, resp.Tip, testTree.Dag.Tip)

	// changing auths should change the owner

	newOwnerKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	newOwner := consensus.EcdsaToPublicKey(&newOwnerKey.PublicKey)

	unsignedBlock = &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: testTree.Dag.Tip.String(),
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]interface{}{
						"path": "_qc/authentications",
						"value": []*consensus.PublicKey{
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

	resp, err = signer.ProcessAddBlock(req)
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

	resp, err = signer.ProcessAddBlock(req)
	assert.NotNil(t, err)

	// however if we sign it with the new owner, it should be accepted.

	blockWithHeaders, err = consensus.SignBlock(unsignedBlock, newOwnerKey)
	assert.Nil(t, err)

	req = &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      emptyTree.Tip,
		NewBlock: blockWithHeaders,
	}

	resp, err = signer.ProcessAddBlock(req)
	assert.Nil(t, err)

	valid, err = testTree.ProcessBlock(blockWithHeaders)
	assert.True(t, valid)
	assert.Nil(t, err)

	assert.Equal(t, resp.Tip, testTree.Dag.Tip)

	val, _, err := testTree.Dag.Resolve([]string{"tree", "another", "path"})
	assert.Nil(t, err)
	assert.Equal(t, "test", val)
}

func TestSigner_ProcessFeedback(t *testing.T) {
	//log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(log.LvlDebug), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

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

	resp, err := signer.ProcessAddBlock(req)

	assert.Nil(t, err)

	testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	testTree.ProcessBlock(blockWithHeaders)

	assert.Equal(t, resp.Tip, testTree.Dag.Tip)

	// above was setup, now testing feedback

	groupSig, err := signer.Group.CombineSignatures(consensus.SignatureMap{resp.SignerId: resp.Signature})
	assert.Nil(t, err)

	feedbackMessage := &consensus.FeedbackRequest{
		Tip:       resp.Tip,
		Signature: *groupSig,
		ChainId:   resp.ChainId,
	}

	err = signer.ProcessFeedback(feedbackMessage)
	assert.Nil(t, err)
}

func TestSigner_ProcessTipRequest(t *testing.T) {
	//log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(log.LvlDebug), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

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

	resp, err := signer.ProcessAddBlock(req)

	assert.Nil(t, err)

	testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)
	assert.Nil(t, err)

	testTree.ProcessBlock(blockWithHeaders)

	assert.Equal(t, resp.Tip, testTree.Dag.Tip)

	groupSig, err := signer.Group.CombineSignatures(consensus.SignatureMap{resp.SignerId: resp.Signature})
	assert.Nil(t, err)

	feedbackMessage := &consensus.FeedbackRequest{
		Tip:       resp.Tip,
		Signature: *groupSig,
		ChainId:   resp.ChainId,
	}

	err = signer.ProcessFeedback(feedbackMessage)
	assert.Nil(t, err)

	// above was setup, now testing tip

	tipMessage := &consensus.TipRequest{
		ChainId: resp.ChainId,
	}

	tipResp, err := signer.ProcessTipRequest(tipMessage)
	assert.Nil(t, err)

	assert.Equal(t, resp.Tip, tipResp.Tip)
}
