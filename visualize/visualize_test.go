// +build visualize

package visualize

// Run this with `go test -v ./visualize/ -tags=visualize`
// then this will output  in ./visualize/.tmp/graph.dot
// you can then run: dot -Tpng -o ./visualize/.tmp/graph.png ./visualize/.tmp/graph.dot
// which will show you the progression of a transaction through the nodes

import (
	"context"
	"crypto/ecdsa"
	"io/ioutil"
	"math/rand"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/awalterschulze/gographviz"
	"github.com/ethereum/go-ethereum/crypto"
	logging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/bls"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/quorumcontrol/tupelo/gossip2"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/require"
)

func dagToByteNodes(t *testing.T, dagTree *dag.Dag) [][]byte {
	cborNodes, err := dagTree.Nodes()
	require.Nil(t, err)
	nodes := make([][]byte, len(cborNodes))
	for i, node := range cborNodes {
		nodes[i] = node.RawData()
	}
	return nodes
}

func newBootstrapHost(ctx context.Context, t *testing.T) *p2p.Host {
	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	host, err := p2p.NewHost(ctx, key, 0)

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
	PubKeys           []consensus.PublicKey
	SignKeysByAddress map[string]*bls.SignKey
}

func newTestSet(t *testing.T, size int) *testSet {
	signKeys := blsKeys(size)
	verKeys := make([]*bls.VerKey, len(signKeys))
	pubKeys := make([]consensus.PublicKey, len(signKeys))
	ecdsaKeys := make([]*ecdsa.PrivateKey, len(signKeys))
	signKeysByAddress := make(map[string]*bls.SignKey)
	for i, signKey := range signKeys {
		ecdsaKey, err := crypto.GenerateKey()
		if err != nil {
			t.Fatalf("error generating key: %v", err)
		}
		verKeys[i] = signKey.MustVerKey()
		pubKeys[i] = consensus.BlsKeyToPublicKey(verKeys[i])
		ecdsaKeys[i] = ecdsaKey
		signKeysByAddress[consensus.BlsVerKeyToAddress(verKeys[i].Bytes()).String()] = signKey

	}

	return &testSet{
		SignKeys:          signKeys,
		VerKeys:           verKeys,
		PubKeys:           pubKeys,
		EcdsaKeys:         ecdsaKeys,
		SignKeysByAddress: signKeysByAddress,
	}
}

func groupFromTestSet(t *testing.T, set *testSet) *consensus.NotaryGroup {
	members := make([]*consensus.RemoteNode, len(set.SignKeys))
	for i := range set.SignKeys {
		rn := consensus.NewRemoteNode(consensus.BlsKeyToPublicKey(set.VerKeys[i]), consensus.EcdsaToPublicKey(&set.EcdsaKeys[i].PublicKey))
		members[i] = rn
	}

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	group := consensus.NewNotaryGroup("notarygroupid", nodeStore)
	err := group.CreateGenesisState(group.RoundAt(time.Now()), members...)
	require.Nil(t, err)
	return group
}

func blsKeys(size int) []*bls.SignKey {
	keys := make([]*bls.SignKey, size)
	for i := 0; i < size; i++ {
		keys[i] = bls.MustNewSignKey()
	}
	return keys
}

func randBytes(length int) []byte {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic("couldn't generate random bytes")
	}
	return b
}

func newValidTransaction(t *testing.T) gossip2.Transaction {
	sw := safewrap.SafeWrap{}
	treeKey, err := crypto.GenerateKey()
	require.Nil(t, err)

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

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	emptyTree := consensus.NewEmptyTree(treeDID, nodeStore)
	emptyTip := emptyTree.Tip
	testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)
	require.Nil(t, err)

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	require.Nil(t, err)

	testTree.ProcessBlock(blockWithHeaders)
	nodes := dagToByteNodes(t, emptyTree)

	req := &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      &emptyTree.Tip,
		NewBlock: blockWithHeaders,
	}
	return gossip2.Transaction{
		PreviousTip: emptyTip.Bytes(),
		NewTip:      testTree.Dag.Tip.Bytes(),
		Payload:     sw.WrapObject(req).RawData(),
		ObjectID:    []byte(treeDID),
	}
}

func NewTestCluster(t *testing.T, groupSize int, ctx context.Context) []*gossip2.GossipNode {
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
	}

	return gossipNodes
}

func TestGossip(t *testing.T) {
	graph := gographviz.NewGraph()
	graph.Directed = true
	logging.SetLogLevel("gossip", "ERROR")
	groupSize := 20

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gossipNodes := NewTestCluster(t, groupSize, ctx)

	transaction1 := newValidTransaction(t)

	reporter := gossip2.NewInMemoryReporter()

	for i := 0; i < groupSize; i++ {
		gossipNodes[i].GossipReporter = reporter
		defer gossipNodes[i].Stop()
		go gossipNodes[i].Start()
	}

	for i := 0; i < 100; i++ {
		_, err := gossipNodes[rand.Intn(len(gossipNodes))].InitiateTransaction(newValidTransaction(t))
		if err != nil {
			t.Fatalf("error sending transaction: %v", err)
		}
	}

	start := time.Now()
	_, err := gossipNodes[0].InitiateTransaction(transaction1)
	require.Nil(t, err)

	for {
		if (time.Now().Sub(start)) > (60 * time.Second) {
			t.Fatal("timed out looking for done function")
			break
		}
		exists, err := gossipNodes[0].Storage.Exists(transaction1.ToConflictSet().DoneID())
		require.Nil(t, err)
		if exists {
			break
		}
	}

	events := reporter.GetSeenEventsFor(transaction1.StoredID())
	for _, event := range events {
		graph.AddNode("G", "P"+event.Seer, nil)
		graph.AddNode("G", "P"+event.Sender, nil)
		err = graph.AddEdge("P"+event.Sender, "P"+event.Seer, true, map[string]string{"label": "\"" + event.When.Sub(start).String() + "\""})
		require.Nil(t, err)
	}

	os.MkdirAll(".tmp", 0755)

	ioutil.WriteFile(".tmp/graph.dot", []byte(graph.String()), 0644)

}
