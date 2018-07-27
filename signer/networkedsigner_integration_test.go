// +build integration

package signer

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegrationNetworkedSignerBuildingTransactions(t *testing.T) {
	//log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(log.LvlDebug), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	blsKey := bls.MustNewSignKey()
	pubKey := consensus.BlsKeyToPublicKey(blsKey.MustVerKey())

	ecdsaKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	group := consensus.NewGroup([]*consensus.RemoteNode{consensus.NewRemoteNode(pubKey, consensus.EcdsaToPublicKey(&ecdsaKey.PublicKey))})

	store := storage.NewMemStorage()

	sign := &Signer{
		Group:   group,
		Id:      consensus.BlsVerKeyToAddress(blsKey.MustVerKey().Bytes()).String(),
		SignKey: blsKey,
		VerKey:  blsKey.MustVerKey(),
	}

	node := network.NewNode(ecdsaKey)

	networkedSigner := NewNetworkedSigner(node, sign, store)

	networkedSigner.Start()

	sessionKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	client := network.NewMessageHandler(network.NewNode(sessionKey), []byte(group.Id()))

	client.Start()
	defer client.Stop()
	time.Sleep(2 * time.Second)

	treeKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	tree, err := consensus.NewSignedChainTree(treeKey.PublicKey)
	assert.Nil(t, err)

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

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	nodes := make([][]byte, len(tree.ChainTree.Dag.Nodes()))
	for i, node := range tree.ChainTree.Dag.Nodes() {
		nodes[i] = node.Node.RawData()
	}

	addBlockRequest := &consensus.AddBlockRequest{
		Nodes:    nodes,
		NewBlock: blockWithHeaders,
		Tip:      tree.Tip(),
	}

	log.Debug("sending: ", "tip", addBlockRequest.Tip, "nodeLength", len(nodes))

	req, err := network.BuildRequest(consensus.MessageType_AddBlock, addBlockRequest)

	respChan, err := client.Broadcast([]byte(group.Id()), crypto.Keccak256([]byte(group.Id())), req)
	assert.Nil(t, err)

	isValid, err := tree.ChainTree.ProcessBlock(blockWithHeaders)
	require.Nil(t, err)
	require.True(t, isValid)

	resp := <-respChan
	require.NotNil(t, resp)

	addResponse := &consensus.AddBlockResponse{}
	err = cbornode.DecodeInto(resp.Payload, addResponse)
	require.Nil(t, err)

	assert.True(t, addResponse.Tip.Equals(tree.ChainTree.Dag.Tip))

	// now let's add a new block

	unsignedBlock2 := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: addResponse.Tip.String(),
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]string{
						"path":  "down/in/the/thing2",
						"value": "hi2",
					},
				},
			},
		},
	}

	blockWithHeaders2, err := consensus.SignBlock(unsignedBlock2, treeKey)
	assert.Nil(t, err)

	nodes2 := make([][]byte, len(tree.ChainTree.Dag.Nodes()))
	for i, node := range tree.ChainTree.Dag.Nodes() {
		nodes2[i] = node.Node.RawData()
	}

	addBlockRequest2 := &consensus.AddBlockRequest{
		Nodes:    nodes2,
		NewBlock: blockWithHeaders2,
		Tip:      tree.Tip(),
	}

	log.Debug("sending: ", "tip", addBlockRequest2.Tip, "nodeLength", len(nodes))

	req2, err := network.BuildRequest(consensus.MessageType_AddBlock, addBlockRequest2)

	respChan2, err := client.Broadcast([]byte(group.Id()), crypto.Keccak256([]byte(group.Id())), req2)
	assert.Nil(t, err)

	isValid, err = tree.ChainTree.ProcessBlock(blockWithHeaders2)
	require.Nil(t, err)
	require.True(t, isValid)

	resp2 := <-respChan2
	require.NotNil(t, resp2)

	addResponse2 := &consensus.AddBlockResponse{}
	err = cbornode.DecodeInto(resp2.Payload, addResponse2)
	require.Nil(t, err)

	assert.True(t, addResponse2.Tip.Equals(tree.ChainTree.Dag.Tip))

	// and fail with a bad tip (tip from first transaction):

	unsignedBlock3 := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: addResponse.Tip.String(),
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]string{
						"path":  "down/in/the/thing2",
						"value": "hi2",
					},
				},
			},
		},
	}

	blockWithHeaders3, err := consensus.SignBlock(unsignedBlock3, treeKey)
	assert.Nil(t, err)

	nodes3 := make([][]byte, len(tree.ChainTree.Dag.Nodes()))
	for i, node := range tree.ChainTree.Dag.Nodes() {
		nodes3[i] = node.Node.RawData()
	}

	addBlockRequest3 := &consensus.AddBlockRequest{
		Nodes:    nodes3,
		NewBlock: blockWithHeaders3,
		Tip:      tree.Tip(),
	}

	log.Debug("sending: ", "tip", addBlockRequest3.Tip, "nodeLength", len(nodes))

	req3, err := network.BuildRequest(consensus.MessageType_AddBlock, addBlockRequest3)

	respChan3, err := client.Broadcast([]byte(group.Id()), crypto.Keccak256([]byte(group.Id())), req3)
	assert.Nil(t, err)

	resp3 := <-respChan3
	require.NotNil(t, resp3)

	assert.Equal(t, 500, resp3.Code)

}

func TestIntegrationNetworkedSigner(t *testing.T) {

	//log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(log.LvlDebug), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	blsKey := bls.MustNewSignKey()
	pubKey := consensus.BlsKeyToPublicKey(blsKey.MustVerKey())

	ecdsaKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	group := consensus.NewGroup([]*consensus.RemoteNode{consensus.NewRemoteNode(pubKey, consensus.EcdsaToPublicKey(&ecdsaKey.PublicKey))})

	store := storage.NewMemStorage()

	sign := &Signer{
		Group:   group,
		Id:      consensus.BlsVerKeyToAddress(blsKey.MustVerKey().Bytes()).String(),
		SignKey: blsKey,
		VerKey:  blsKey.MustVerKey(),
	}

	node := network.NewNode(ecdsaKey)

	networkedSigner := NewNetworkedSigner(node, sign, store)

	networkedSigner.Start()

	sessionKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	client := network.NewMessageHandler(network.NewNode(sessionKey), []byte(group.Id()))

	client.Start()
	defer client.Stop()
	time.Sleep(2 * time.Second)

	treeKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	tree, err := consensus.NewSignedChainTree(treeKey.PublicKey)
	assert.Nil(t, err)

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

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	assert.Nil(t, err)

	nodes := make([][]byte, len(tree.ChainTree.Dag.Nodes()))
	for i, node := range tree.ChainTree.Dag.Nodes() {
		nodes[i] = node.Node.RawData()
	}

	addBlockRequest := &consensus.AddBlockRequest{
		Nodes:    nodes,
		NewBlock: blockWithHeaders,
		Tip:      tree.Tip(),
	}

	log.Debug("sending: ", "tip", addBlockRequest.Tip, "nodeLength", len(nodes))

	req, err := network.BuildRequest(consensus.MessageType_AddBlock, addBlockRequest)

	respChan, err := client.Broadcast([]byte(group.Id()), crypto.Keccak256([]byte(group.Id())), req)
	assert.Nil(t, err)

	isValid, err := tree.ChainTree.ProcessBlock(blockWithHeaders)
	require.Nil(t, err)
	require.True(t, isValid)

	resp := <-respChan
	require.NotNil(t, resp)

	addResponse := &consensus.AddBlockResponse{}
	err = cbornode.DecodeInto(resp.Payload, addResponse)
	require.Nil(t, err)

	assert.True(t, addResponse.Tip.Equals(tree.ChainTree.Dag.Tip))

	// now let's process a feedback message

	groupSig, err := group.CombineSignatures(consensus.SignatureMap{addResponse.SignerId: addResponse.Signature})
	require.Nil(t, err)

	feedbackMessage := &consensus.FeedbackRequest{
		Tip:       addResponse.Tip,
		Signature: *groupSig,
		ChainId:   addResponse.ChainId,
	}

	feedbackReq, err := network.BuildRequest(consensus.MessageType_Feedback, feedbackMessage)
	require.Nil(t, err)

	feedbackRespChan, err := client.Broadcast([]byte(group.Id()), crypto.Keccak256([]byte(group.Id())), feedbackReq)
	require.Nil(t, err)

	feedbackResp := <-feedbackRespChan
	require.NotNil(t, feedbackResp)

	// and then get the tip

	tipMessage := &consensus.TipRequest{
		ChainId: addResponse.ChainId,
	}

	tipReq, err := network.BuildRequest(consensus.MessageType_TipRequest, tipMessage)

	tipRespChan, err := client.Broadcast([]byte(group.Id()), crypto.Keccak256([]byte(group.Id())), tipReq)
	require.Nil(t, err)

	tipRespMsg := <-tipRespChan
	require.NotNil(t, tipRespMsg)

	tipResponse := &consensus.TipResponse{}
	err = cbornode.DecodeInto(tipRespMsg.Payload, tipResponse)
	require.Nil(t, err)

	assert.True(t, tipResponse.Tip.Equals(addResponse.Tip))

}
