// +build integration

package signer

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/stretchr/testify/assert"
)

func TestIntegrationNetworkedSigner(t *testing.T) {

	//log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(log.LvlDebug), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	blsKey := bls.MustNewSignKey()
	pubKey := consensus.BlsKeyToPublicKey(blsKey.MustVerKey())

	ecdsaKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	group := consensus.NewGroup([]*consensus.RemoteNode{consensus.NewRemoteNode(pubKey, consensus.EcdsaToPublicKey(&ecdsaKey.PublicKey))})

	store := storage.NewMemStorage()

	sign := &Signer{
		Storage: store,
		Group:   group,
		Id:      consensus.BlsVerKeyToAddress(blsKey.MustVerKey().Bytes()).String(),
		SignKey: blsKey,
		VerKey:  blsKey.MustVerKey(),
	}

	sign.SetupStorage()

	node := network.NewNode(ecdsaKey)

	networkedSigner := NewNetworkedSigner(node, sign)
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

	log.Debug("sending: ", "tip", addBlockRequest.Tip, "len(nodes)", len(nodes))

	req, err := network.BuildRequest(consensus.MessageType_AddBlock, addBlockRequest)

	respChan, err := client.Broadcast([]byte(group.Id()), crypto.Keccak256([]byte(group.Id())), req)
	assert.Nil(t, err)

	resp := <-respChan

	assert.NotNil(t, resp)

}
