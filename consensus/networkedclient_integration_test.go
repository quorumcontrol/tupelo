// +build integration

package consensus_test

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/signer"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/stretchr/testify/assert"
)

func TestNetworkedClient_AddBlock(t *testing.T) {
	blsKey := bls.MustNewSignKey()
	pubKey := consensus.BlsKeyToPublicKey(blsKey.MustVerKey())

	ecdsaKey, err := crypto.GenerateKey()
	assert.Nil(t, err)
	dstPubKey := consensus.EcdsaToPublicKey(&ecdsaKey.PublicKey)

	rn := consensus.NewRemoteNode(pubKey, dstPubKey)

	group := consensus.NewGroup([]*consensus.RemoteNode{rn})

	store := storage.NewMemStorage()

	sign := &signer.Signer{
		Group:   group,
		Id:      consensus.BlsVerKeyToAddress(blsKey.MustVerKey().Bytes()).String(),
		SignKey: blsKey,
		VerKey:  blsKey.MustVerKey(),
	}

	node := network.NewNode(ecdsaKey)

	networkedSigner := signer.NewNetworkedSigner(node, sign, store)
	networkedSigner.Start()

	client, err := consensus.NewNetworkedClient(group)
	assert.Nil(t, err)
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

	resp, err := client.AddBlock(tree, blockWithHeaders)

	assert.Nil(t, err)

	valid, err := group.VerifySignature(consensus.MustObjToHash(resp.Tip.Bytes()), &resp.Signature)
	assert.Nil(t, err)
	assert.True(t, valid)

	// then getting the tip works (because feedback went through)

	id, err := tree.Id()
	assert.Nil(t, err)

	tipResp, err := client.RequestTip(id)
	assert.Nil(t, err)

	assert.Equal(t, tipResp.Tip, resp.Tip)
}

func BenchmarkNetworkedClient_AddBlock(b *testing.B) {
	blsKey := bls.MustNewSignKey()
	pubKey := consensus.BlsKeyToPublicKey(blsKey.MustVerKey())

	ecdsaKey, err := crypto.GenerateKey()
	assert.Nil(b, err)
	dstPubKey := consensus.EcdsaToPublicKey(&ecdsaKey.PublicKey)

	rn := consensus.NewRemoteNode(pubKey, dstPubKey)

	group := consensus.NewGroup([]*consensus.RemoteNode{rn})

	store := storage.NewMemStorage()

	sign := &signer.Signer{
		Group:   group,
		Id:      consensus.BlsVerKeyToAddress(blsKey.MustVerKey().Bytes()).String(),
		SignKey: blsKey,
		VerKey:  blsKey.MustVerKey(),
	}

	node := network.NewNode(ecdsaKey)

	networkedSigner := signer.NewNetworkedSigner(node, sign, store)
	networkedSigner.Start()

	client, err := consensus.NewNetworkedClient(group)
	assert.Nil(b, err)
	client.Start()
	defer client.Stop()
	time.Sleep(2 * time.Second)

	treeKey, err := crypto.GenerateKey()
	assert.Nil(b, err)

	tree, err := consensus.NewSignedChainTree(treeKey.PublicKey)
	assert.Nil(b, err)

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
	assert.Nil(b, err)

	b.ResetTimer()
	for n := 0; n < b.N; n++ {
		_, err = client.AddBlock(tree, blockWithHeaders)
	}

	assert.Nil(b, err)
}
