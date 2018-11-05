// +build integration

package gossipclient

import (
	"crypto/ecdsa"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/signer"
	"github.com/quorumcontrol/storage"
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

func TestIntegrationGossipClient(t *testing.T) {
	ts := newTestSet(t, 5)

	store := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	remoteNodes := []*consensus.RemoteNode{consensus.NewRemoteNode(ts.PubKeys[0], ts.DstKeys[0])}
	group := consensus.NewNotaryGroup("notaryGroupId", store)
	err := group.CreateGenesisState(group.RoundAt(time.Now()), remoteNodes...)
	require.Nil(t, err)
	node1 := network.NewNode(ts.EcdsaKeys[0])
	store1 := storage.NewMemStorage()

	gossipedSigner1 := signer.NewGossipedSigner(node1, group, store1, ts.SignKeys[0])

	gossipedSigner1.Start()
	defer gossipedSigner1.Stop()

	client := NewGossipClient(group)
	client.Start()
	defer client.Stop()

	treeKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	tree, err := consensus.NewSignedChainTree(treeKey.PublicKey, store)
	assert.Nil(t, err)

	resp, err := client.PlayTransactions(tree, treeKey, "", []*chaintree.Transaction{
		{
			Type: "SET_DATA",
			Payload: map[string]string{
				"path":  "down/in/the/thing",
				"value": "hi",
			},
		},
	})

	require.Nil(t, err)

	// test that the remote tip matches the local tip
	assert.True(t, tree.Tip().Equals(*resp.Tip), "local: %s, remote: %s", tree.Tip(), resp.Tip)

	// test it modifies the local tree
	val, _, err := tree.ChainTree.Dag.Resolve(strings.Split("tree/down/in/the/thing", "/"))
	assert.Nil(t, err)
	assert.Equal(t, "hi", val)

	assert.Equal(t, resp.Signature, tree.Signatures[group.ID])

	// now get the tip and make sure it maches the response

	tipResp, err := client.TipRequest(tree.MustId())
	require.Nil(t, err)
	t.Logf("tipResp: %v", tipResp)

	assert.True(t, tree.Tip().Equals(*tipResp.Tip), "local: %s, remote: %s", tree.Tip(), resp.Tip)

}
