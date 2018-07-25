// +build integration

package signer

import (
	"crypto/ecdsa"
	"testing"

	"time"

	"os"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testSet struct {
	SignKeys          []*bls.SignKey
	VerKeys           []*bls.VerKey
	EcdsaKeys         []*ecdsa.PrivateKey
	DstKeys           []consensus.PublicKey
	PubKeys           []consensus.PublicKey
	SignKeysByAddress map[string]*bls.SignKey
}

func blsKeys(size int) []*bls.SignKey {
	keys := make([]*bls.SignKey, size)
	for i := 0; i < size; i++ {
		keys[i] = bls.MustNewSignKey()
	}
	return keys
}

func newTestSet(t *testing.T, size int) *testSet {
	signKeys := blsKeys(size)
	verKeys := make([]*bls.VerKey, len(signKeys))
	pubKeys := make([]consensus.PublicKey, len(signKeys))
	ecdsaKeys := make([]*ecdsa.PrivateKey, len(signKeys))
	dstKeys := make([]consensus.PublicKey, len(signKeys))
	signKeysByAddress := make(map[string]*bls.SignKey)
	for i, signKey := range signKeys {
		ecdsaKey, _ := crypto.GenerateKey()
		verKeys[i] = signKey.MustVerKey()
		pubKeys[i] = consensus.BlsKeyToPublicKey(verKeys[i])
		ecdsaKeys[i] = ecdsaKey
		dstKeys[i] = consensus.EcdsaToPublicKey(&ecdsaKey.PublicKey)
		signKeysByAddress[consensus.BlsVerKeyToAddress(verKeys[i].Bytes()).String()] = signKey

	}

	return &testSet{
		SignKeys:          signKeys,
		VerKeys:           verKeys,
		PubKeys:           pubKeys,
		EcdsaKeys:         ecdsaKeys,
		DstKeys:           dstKeys,
		SignKeysByAddress: signKeysByAddress,
	}
}

func sendBlock(t *testing.T, signed *chaintree.BlockWithHeaders, tip *cid.Cid, tree *consensus.SignedChainTree, client *network.MessageHandler, dst *ecdsa.PublicKey) network.ResponseChan {

	nodes := make([][]byte, len(tree.ChainTree.Dag.Nodes()))
	for i, node := range tree.ChainTree.Dag.Nodes() {
		nodes[i] = node.Node.RawData()
	}

	addBlockRequest := &consensus.AddBlockRequest{
		ChainId:  tree.MustId(),
		Nodes:    nodes,
		NewBlock: signed,
		Tip:      tip,
	}

	req, err := network.BuildRequest(consensus.MessageType_AddBlock, addBlockRequest)
	require.Nil(t, err)

	respChan, err := client.DoRequest(dst, req)
	require.Nil(t, err)
	return respChan
}

func TestGossipedSignerIntegration(t *testing.T) {
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(log.LvlDebug), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	ts := newTestSet(t, 5)
	remoteNodes := []*consensus.RemoteNode{consensus.NewRemoteNode(ts.PubKeys[0], ts.DstKeys[0])}
	group := consensus.NewGroup(remoteNodes)

	signer1 := &Signer{
		Group:   group,
		Id:      consensus.BlsVerKeyToAddress(ts.VerKeys[0].Bytes()).String(),
		SignKey: ts.SignKeys[0],
		VerKey:  ts.SignKeys[0].MustVerKey(),
	}

	node1 := network.NewNode(ts.EcdsaKeys[0])
	store1 := storage.NewMemStorage()

	gossipedSigner1 := NewGossipedSigner(node1, signer1, store1)

	gossipedSigner1.Start()
	defer gossipedSigner1.Stop()

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

	// First we test that a gossipedSigner1 can receive messages

	signed, err := consensus.SignBlock(&chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				{
					Type: consensus.TransactionTypeSetData,
					Payload: consensus.SetDataPayload{
						Path:  "down/in/the/thing",
						Value: "hi",
					},
				},
			},
		}}, treeKey)

	respChan := sendBlock(t, signed, tree.Tip(), tree, client, &ts.EcdsaKeys[0].PublicKey)

	respBytes := <-respChan
	assert.NotNil(t, respBytes)

	// now we check that we can stake in order to become part of the group
	tree.ChainTree.ProcessBlock(signed)
	log.Debug("expected tip: ", "tip", tree.Tip().String())

	signer2 := &Signer{
		Group:   group,
		Id:      consensus.BlsVerKeyToAddress(ts.VerKeys[1].Bytes()).String(),
		SignKey: ts.SignKeys[1],
		VerKey:  ts.SignKeys[1].MustVerKey(),
	}

	node2 := network.NewNode(ts.EcdsaKeys[1])
	store2 := storage.NewMemStorage()

	gossipedSigner2 := NewGossipedSigner(node2, signer2, store2)
	gossipedSigner2.Start()
	defer gossipedSigner2.Stop()

	stakeBlock, err := consensus.SignBlock(&chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: tree.ChainTree.Dag.Tip.String(),
			Transactions: []*chaintree.Transaction{
				{
					Type: consensus.TransactionTypeStake,
					Payload: consensus.StakePayload{
						DstKey:  ts.DstKeys[1],
						VerKey:  ts.PubKeys[1],
						GroupId: group.Id(),
					},
				},
			},
		},
	}, treeKey)
	require.Nil(t, err)
	respChan = sendBlock(t, stakeBlock, tree.Tip(), tree, client, &ts.EcdsaKeys[0].PublicKey)

	stakeRespBytes := <-respChan
	assert.NotNil(t, stakeRespBytes)

}
