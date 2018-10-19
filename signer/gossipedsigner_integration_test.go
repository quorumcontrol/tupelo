// +build integration

package signer

import (
	"context"
	"crypto/ecdsa"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/gossipclient"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/p2p"
	"github.com/quorumcontrol/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBootstrapHost(ctx context.Context, t *testing.T) *p2p.Host {
	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	host, err := p2p.NewHost(ctx, key, p2p.GetRandomUnusedPort())

	require.Nil(t, err)
	require.NotNil(t, host)

	return host
}

func bootstrapAddresses(bootstrapHost *p2p.Host) []string {
	addresses := bootstrapHost.Addresses()
	for _, addr := range addresses {
		addrStr := addr.String()
		if strings.Contains(addrStr, "127.0.0.1") {
			return []string{addrStr}
		}
	}
	return nil
}

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
	cborNodes, err := tree.ChainTree.Dag.Nodes()
	require.Nil(t, err)
	nodes := make([][]byte, len(cborNodes))
	for i, node := range cborNodes {
		nodes[i] = node.RawData()
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

func notaryGroupFromRemoteNodes(t *testing.T, remoteNodes []*consensus.RemoteNode) *consensus.NotaryGroup {
	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	group := consensus.NewNotaryGroup("notarygroupID", nodeStore)
	group.RoundLength = 3
	err := group.CreateGenesisState(group.RoundAt(time.Now()), remoteNodes...)
	require.Nil(t, err)
	return group
}

func TestGossipedSignerIntegration(t *testing.T) {
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(log.LvlDebug), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	boostrapHost := newBootstrapHost(ctx, t)

	ts := newTestSet(t, 5)
	remoteNodes := []*consensus.RemoteNode{consensus.NewRemoteNode(ts.PubKeys[0], ts.DstKeys[0])}

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())

	group := notaryGroupFromRemoteNodes(t, remoteNodes)

	node1 := network.NewNode(ts.EcdsaKeys[0])
	node1.BoostrapNodes = bootstrapAddresses(boostrapHost)
	store1 := storage.NewMemStorage()

	gossipedSigner1 := NewGossipedSigner(node1, group, store1, ts.SignKeys[0])

	gossipedSigner1.Start()
	defer gossipedSigner1.Stop()

	sessionKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	clientNode := network.NewNode(sessionKey)
	clientNode.BoostrapNodes = bootstrapAddresses(boostrapHost)

	client := network.NewMessageHandler(clientNode, []byte(group.ID))

	client.Start()
	defer client.Stop()

	treeKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	tree, err := consensus.NewSignedChainTree(treeKey.PublicKey, nodeStore)
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

	tree.ChainTree.ProcessBlock(signed)
	log.Debug("expected tip: ", "tip", tree.Tip().String())

	roundsAndExpectedCount := map[int64]int{}
	var lastRound int64

	// now we check that we can stake in order to become part of the group
	for i := 1; i <= 2; i++ {
		nextTreeKey, err := crypto.GenerateKey()
		assert.Nil(t, err)
		nextNodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
		nextTree, err := consensus.NewSignedChainTree(nextTreeKey.PublicKey, nextNodeStore)

		nextNode := network.NewNode(ts.EcdsaKeys[i+1])
		nextStore := storage.NewMemStorage()
		nextSigner := NewGossipedSigner(nextNode, group, nextStore, ts.SignKeys[i+1])
		nextSigner.Start()
		defer nextSigner.Stop()

		<-time.After(time.Duration(group.RoundLength) * time.Second)

		stakeBlock, err := consensus.SignBlock(&chaintree.BlockWithHeaders{
			Block: chaintree.Block{
				PreviousTip: "",
				Transactions: []*chaintree.Transaction{
					{
						Type: consensus.TransactionTypeStake,
						Payload: consensus.StakePayload{
							DstKey:  ts.DstKeys[i+1],
							VerKey:  ts.PubKeys[i+1],
							GroupId: group.ID,
						},
					},
				},
			},
		}, nextTreeKey)
		require.Nil(t, err)
		respChan = sendBlock(t, stakeBlock, nextTree.Tip(), nextTree, client, &ts.EcdsaKeys[0].PublicKey)

		stakeRespBytes := <-respChan
		assert.NotNil(t, stakeRespBytes)
		nextTree.ChainTree.ProcessBlock(stakeBlock)

		expectedRound := group.RoundAt(time.Now()) + 6
		roundsAndExpectedCount[expectedRound] = i + 1
		lastRound = expectedRound
	}

	// Give it two rounds to commit
	<-time.After(2 * time.Duration(group.RoundLength) * time.Second)

	for round, expected := range roundsAndExpectedCount {
		roundInfo, err := group.MostRecentRoundInfo(round)
		require.Nil(t, err)
		assert.Len(t, roundInfo.Signers, expected)
	}

	// Check that the round after our last insert persists length of signers
	roundInfo, err := group.MostRecentRoundInfo(lastRound + 1)
	require.Nil(t, err)
	assert.Len(t, roundInfo.Signers, roundsAndExpectedCount[lastRound])
}

func TestGossipedSigner_GetDiffNodesHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	boostrapHost := newBootstrapHost(ctx, t)

	ts := newTestSet(t, 5)
	remoteNodes := []*consensus.RemoteNode{consensus.NewRemoteNode(ts.PubKeys[0], ts.DstKeys[0])}
	group := notaryGroupFromRemoteNodes(t, remoteNodes)

	node1 := network.NewNode(ts.EcdsaKeys[0])
	node1.BoostrapNodes = bootstrapAddresses(boostrapHost)
	store1 := storage.NewMemStorage()

	gossipedSigner1 := NewGossipedSigner(node1, group, store1, ts.SignKeys[0])

	gossipedSigner1.Start()
	defer gossipedSigner1.Stop()

	sessionKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	clientNode := network.NewNode(sessionKey)
	clientNode.BoostrapNodes = bootstrapAddresses(boostrapHost)
	client := network.NewMessageHandler(clientNode, []byte(group.ID))

	client.Start()
	defer client.Stop()

	req, err := network.BuildRequest(consensus.MessageType_GetDiffNodes, &consensus.GetDiffNodesRequest{
		PreviousTip: group.Tip(),
		NewTip:      group.Tip(),
	})
	require.Nil(t, err)

	respChan, err := client.DoRequest(&ts.EcdsaKeys[0].PublicKey, req)
	require.Nil(t, err)

	respBytes := <-respChan
	require.Equal(t, 200, respBytes.Code, "code: %d, payload: %s", respBytes.Code, respBytes.Payload)
}

func TestGossipedSigner_TipHandler(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	boostrapHost := newBootstrapHost(ctx, t)

	ts := newTestSet(t, 5)
	remoteNodes := []*consensus.RemoteNode{consensus.NewRemoteNode(ts.PubKeys[0], ts.DstKeys[0])}
	group := notaryGroupFromRemoteNodes(t, remoteNodes)

	node1 := network.NewNode(ts.EcdsaKeys[0])
	node1.BoostrapNodes = bootstrapAddresses(boostrapHost)
	store1 := storage.NewMemStorage()

	gossipedSigner1 := NewGossipedSigner(node1, group, store1, ts.SignKeys[0])

	gossipedSigner1.Start()
	defer gossipedSigner1.Stop()

	sessionKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	clientNode := network.NewNode(sessionKey)
	clientNode.BoostrapNodes = bootstrapAddresses(boostrapHost)
	client := network.NewMessageHandler(clientNode, []byte(group.ID))

	client.Start()
	defer client.Stop()

	treeKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())

	tree, err := consensus.NewSignedChainTree(treeKey.PublicKey, nodeStore)
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

	// then we request the tip to make sure the current state happened

	tree.ChainTree.ProcessBlock(signed)

	req, err := network.BuildRequest(consensus.MessageType_TipRequest, &consensus.TipRequest{
		ChainId: tree.MustId(),
	})
	require.Nil(t, err)

	respChan, err = client.DoRequest(&ts.EcdsaKeys[0].PublicKey, req)
	require.Nil(t, err)

	respBytes = <-respChan
	require.Equal(t, 200, respBytes.Code, "code: %d, payload: %s", respBytes.Code, respBytes.Payload)

	tipResp := &consensus.TipResponse{}
	err = cbornode.DecodeInto(respBytes.Payload, tipResp)
	require.Nil(t, err)
	require.NotNil(t, tipResp.Tip)

	assert.True(t, tipResp.Tip.Equals(tree.Tip()), "tipResp: %s, tree: %s", tipResp.Tip.String(), tree.Tip().String())
}

func TestGossipedSignerIntegrationMultiNode(t *testing.T) {
	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(log.LvlDebug), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	boostrapHost := newBootstrapHost(ctx, t)

	ts := newTestSet(t, 3)
	remoteNodes := make([]*consensus.RemoteNode, len(ts.SignKeys))

	for i := 0; i < len(ts.SignKeys); i++ {
		remoteNodes[i] = consensus.NewRemoteNode(ts.PubKeys[i], ts.DstKeys[i])
	}

	for i := 0; i < len(ts.SignKeys); i++ {
		group := notaryGroupFromRemoteNodes(t, remoteNodes)
		node := network.NewNode(ts.EcdsaKeys[i])
		node.BoostrapNodes = bootstrapAddresses(boostrapHost)
		store := storage.NewMemStorage()
		gossipedSigner := NewGossipedSigner(node, group, store, ts.SignKeys[i])
		gossipedSigner.Start()
		defer gossipedSigner.Stop()
	}
	cliGroup := notaryGroupFromRemoteNodes(t, remoteNodes)

	client := gossipclient.NewGossipClient(cliGroup, bootstrapAddresses(boostrapHost))

	client.Start()
	defer client.Stop()

	key, err := crypto.GenerateKey()
	require.Nil(t, err)
	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())

	chain, err := consensus.NewSignedChainTree(key.PublicKey, nodeStore)
	require.Nil(t, err)

	resp, err := client.PlayTransactions(chain, key, "", []*chaintree.Transaction{
		{
			Type: consensus.TransactionTypeSetData,
			Payload: consensus.SetDataPayload{
				Path:  "test/path",
				Value: "value",
			},
		},
	})

	require.Nil(t, err)
	assert.True(t, resp.Tip.Equals(chain.Tip()), "resp: %s, chain: %s", resp.Tip.String(), chain.Tip().String())
}
