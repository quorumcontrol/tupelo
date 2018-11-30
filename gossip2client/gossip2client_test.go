package gossip2client

import (
	"context"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	logging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	protocol "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-protocol"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/bls"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/quorumcontrol/tupelo/gossip2"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testStoragePath = ".tmp/storage/"

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

func TestSend(t *testing.T) {
	sessionKey, err := crypto.GenerateKey()
	if err != nil {
		panic("error generating key")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	target, err := p2p.NewHost(ctx, sessionKey, 0)

	protocolToTest := protocol.ID("tupelo-test/v1")
	bytesToTest := []byte("thesearebytestotest")

	respCh := make(chan []byte)

	target.SetStreamHandler(protocolToTest, func(stream net.Stream) {
		resp, err := ioutil.ReadAll(stream)
		require.Nil(t, err)
		respCh <- resp
	})

	client := NewGossipClient(nil, bootstrapAddresses(target))
	// Give the bootstrap 0.01 seconds to get the bootstrap actually figured out
	time.Sleep(100 * time.Millisecond)

	err = client.Send(&sessionKey.PublicKey, protocolToTest, bytesToTest, 30)
	require.Nil(t, err)

	assert.Equal(t, <-respCh, bytesToTest)
}

func TestSubscribe(t *testing.T) {
	logging.SetLogLevel("gossip2client", "DEBUG")
	logging.SetLogLevel("gossip", "DEBUG")

	groupSize := 3
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gossipNodes := make([]*gossip2.GossipNode, groupSize)
	ts := testnotarygroup.NewTestSet(t, groupSize)
	group := testnotarygroup.GroupFromTestSet(t, ts)
	bootstrap := testnotarygroup.NewBootstrapHost(ctx, t)

	for i := 0; i < groupSize; i++ {
		host, err := p2p.NewHost(ctx, ts.EcdsaKeys[i], 0)
		require.Nil(t, err)
		host.Bootstrap(bootstrapAddresses(bootstrap))
		storage := storage.NewMemStorage()
		gossipNodes[i] = gossip2.NewGossipNode(ts.EcdsaKeys[i], ts.SignKeys[i], host, storage)
		gossipNodes[i].Group = group
		go gossipNodes[i].Start()
		defer gossipNodes[i].Stop()
	}

	blsKey, _ := bls.NewSignKey()
	treeKey, _ := crypto.ToECDSA(blsKey.Bytes())
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]string{
						"path":  "down/in/the/thing",
						"value": "sometestvalue",
					},
				},
			},
		},
	}

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	emptyTree := consensus.NewEmptyTree(treeDID, nodeStore)
	emptyTip := emptyTree.Tip
	testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)

	blockWithHeaders, _ := consensus.SignBlock(unsignedBlock, treeKey)
	testTree.ProcessBlock(blockWithHeaders)

	cborNodes, _ := emptyTree.Nodes()
	nodes := make([][]byte, len(cborNodes))
	for i, node := range cborNodes {
		nodes[i] = node.RawData()
	}

	req := &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      &emptyTree.Tip,
		NewBlock: blockWithHeaders,
	}
	sw := safewrap.SafeWrap{}

	trans := gossip2.Transaction{
		PreviousTip: emptyTip.Bytes(),
		NewTip:      testTree.Dag.Tip.Bytes(),
		Payload:     sw.WrapObject(req).RawData(),
		ObjectID:    []byte(treeDID),
	}

	client := NewGossipClient(nil, bootstrapAddresses(bootstrap))

	// Give the bootstrap 0.01 seconds to get the bootstrap actually figured out
	time.Sleep(100 * time.Millisecond)

	stateCh, err := client.Subscribe(&ts.EcdsaKeys[0].PublicKey, treeDID, 5*time.Second)
	require.Nil(t, err)

	encodedTrans, err := trans.MarshalMsg(nil)
	require.Nil(t, err)

	err = client.Send(&ts.EcdsaKeys[1].PublicKey, protocol.ID(gossip2.NewTransactionProtocol), encodedTrans, 5*time.Second)
	require.Nil(t, err)

	subscribeResp := <-stateCh

	require.Nil(t, subscribeResp.Error)
	require.Equal(t, subscribeResp.State.Tip, testTree.Dag.Tip.Bytes())
}

func TestPlayTransactionAndTip(t *testing.T) {
	logging.SetLogLevel("gossip2client", "DEBUG")
	logging.SetLogLevel("gossip", "ERROR")

	groupSize := 3
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gossipNodes := make([]*gossip2.GossipNode, groupSize)
	ts := testnotarygroup.NewTestSet(t, groupSize)
	group := testnotarygroup.GroupFromTestSet(t, ts)
	bootstrap := testnotarygroup.NewBootstrapHost(ctx, t)

	for i := 0; i < groupSize; i++ {
		host, err := p2p.NewHost(ctx, ts.EcdsaKeys[i], 0)
		require.Nil(t, err)
		host.Bootstrap(bootstrapAddresses(bootstrap))
		storage := storage.NewMemStorage()
		gossipNodes[i] = gossip2.NewGossipNode(ts.EcdsaKeys[i], ts.SignKeys[i], host, storage)
		gossipNodes[i].Group = group
		go gossipNodes[i].Start()
		defer gossipNodes[i].Stop()
	}

	treeKey, err := crypto.GenerateKey()
	require.Nil(t, err)
	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	chain, err := consensus.NewSignedChainTree(treeKey.PublicKey, nodeStore)
	client := NewGossipClient(group, bootstrapAddresses(bootstrap))

	// Give the bootstrap 0.01 seconds to get the bootstrap actually figured out
	time.Sleep(100 * time.Millisecond)

	var remoteTip string
	if !chain.IsGenesis() {
		remoteTip = chain.Tip().String()
	}

	resp, err := client.PlayTransactions(chain, treeKey, remoteTip, []*chaintree.Transaction{
		{
			Type: "SET_DATA",
			Payload: map[string]string{
				"path":  "down/in/the/thing",
				"value": "sometestvalue",
			},
		},
	})
	require.Nil(t, err)
	assert.Equal(t, resp.Tip.Bytes(), chain.Tip().Bytes())
}
