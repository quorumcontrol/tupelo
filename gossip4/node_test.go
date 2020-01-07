package gossip4

import (
	"context"
	"fmt"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip4/hamtwrapper"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	logging "github.com/ipfs/go-log"

	"github.com/stretchr/testify/require"

	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"

	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/testhelpers"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip4/types"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
)

func newTupeloSystem(ctx context.Context, testSet *testnotarygroup.TestSet) (*types.NotaryGroup, []*Node, error) {
	nodes := make([]*Node, len(testSet.SignKeys))

	ng := types.NewNotaryGroup("testnotary")
	for i, signKey := range testSet.SignKeys {
		sk := signKey
		signer := types.NewRemoteSigner(testSet.PubKeys[i], sk.MustVerKey())
		ng.AddSigner(signer)
	}

	for i := range ng.AllSigners() {
		p2pNode, peer, err := p2p.NewHostAndBitSwapPeer(ctx, p2p.WithKey(testSet.EcdsaKeys[i])) // TODO: options?
		if err != nil {
			return nil, nil, fmt.Errorf("error making node: %v", err)
		}

		n, err := NewNode(ctx, &NewNodeOptions{
			P2PNode:     p2pNode,
			SignKey:     testSet.SignKeys[i],
			NotaryGroup: ng,
			DagStore:    peer,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("error making node: %v", err)
		}
		nodes[i] = n
	}

	return ng, nodes, nil
}

func startNodes(t *testing.T, ctx context.Context, nodes []*Node) {
	n := nodes[0]
	// logging.SetLogLevel(fmt.Sprintf("node-%d", n.signerIndex), "info")
	bootAddrs := make([]string, len(n.p2pNode.Addresses()))
	for i, addr := range n.p2pNode.Addresses() {
		bootAddrs[i] = addr.String()
	}

	for i, node := range nodes {
		logging.SetLogLevel(fmt.Sprintf("node-%d", node.signerIndex), "INFO")

		if i > 0 {
			err := node.Bootstrap(ctx, bootAddrs)
			require.Nil(t, err)
			defer node.Close()
		}
		err := node.Start(ctx)
		require.Nil(t, err)
	}
}

func waitForAllAbrs(t *testing.T, ctx context.Context, nodes []*Node, abrs []*services.AddBlockRequest) {
	n := nodes[0]

	allIncluded := func() bool {
		current := n.rounds.Current()
		if current.height == 0 {
			return false
		}

		for _, abr := range abrs {
			did := string(abr.ObjectId)
			var tip cid.Cid
			r, _ := n.rounds.Get(current.height - 1)
			err := r.state.Find(ctx, did, &tip)
			if err == hamt.ErrNotFound {
				return false
			}
			require.Nil(t, err)
		}
		return true
	}

	// wait for all transCount transactions to be included in the currentCommit
	timer := time.NewTimer(10 * time.Second)
looper:
	for {
		select {
		case <-timer.C:
			t.Fatalf("timeout waiting for all transactions")
		default:
			// do nothing
		}
		if allIncluded() {
			break looper
		}
		time.Sleep(100 * time.Millisecond)
	}
	timer.Stop()
}

func TestNewNode(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	numMembers := 1
	ts := testnotarygroup.NewTestSet(t, numMembers)
	_, nodes, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)
	require.Len(t, nodes, numMembers)
	n := nodes[0]
	err = n.Start(ctx)
	require.Nil(t, err)

	abr, err := n.getCurrent(ctx, "no way")
	require.Nil(t, abr)
	require.Nil(t, err)
}

func TestEndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	numMembers := 3
	ts := testnotarygroup.NewTestSet(t, numMembers)
	_, nodes, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)
	require.Len(t, nodes, numMembers)

	startNodes(t, ctx, nodes)

	abrCount := 10
	abrs := make([]*services.AddBlockRequest, abrCount)

	testStore := hamtwrapper.DagStoreToCborIpld(nodestore.MustMemoryStore(ctx))

	for i := 0; i < abrCount; i++ {
		abr := testhelpers.NewValidTransaction(t)

		id, err := testStore.Put(ctx, abr)
		require.Nil(t, err)
		t.Logf("transaction %d has cid %s", i, id.String())
		time.Sleep(time.Duration((1000 / abrCount)) * time.Millisecond)
		abrs[i] = &abr
	}

	for i, abr := range abrs {
		bits, err := abr.Marshal()
		require.Nil(t, err)

		nodes[i%(len(nodes)-1)].pubsub.Publish(transactionTopic, bits)
		require.Nil(t, err)
	}

	waitForAllAbrs(t, ctx, nodes, abrs)
}

func TestByzantineCases(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	numMembers := 3
	ts := testnotarygroup.NewTestSet(t, numMembers)
	_, nodes, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)
	require.Len(t, nodes, numMembers)

	startNodes(t, ctx, nodes)

	t.Run("conflicting chaintree blocks", func(t *testing.T) {
		// use a canonical key because that influences the hash of the ABRs
		keyHex := "0xcd24af4d6c47530202f00442282fa23e06c1adea93e0264cacabf274241918d2"
		treeKey, err := crypto.ToECDSA(hexutil.MustDecode(keyHex))
		require.Nil(t, err)

		abr1 := testhelpers.NewValidTransactionWithPathAndValue(t, treeKey, "/path", "value")
		abr2 := testhelpers.NewValidTransactionWithPathAndValue(t, treeKey, "/path", "differentvalue")

		// abr1 and abr2 are conflicting blocks added to the same chaintree
		require.Equal(t, abr1.ObjectId, abr2.ObjectId)
		require.Equal(t, abr1.Height, abr2.Height)
		require.NotEqual(t, abr1.NewTip, abr2.NewTip)
		abr1Cid := abrToHamtCID(ctx, &abr1)
		abr2Cid := abrToHamtCID(ctx, &abr2)
		t.Logf("abr1: %s, abr2: %s", abr1Cid.String(), abr2Cid.String())

		abrs := []*services.AddBlockRequest{&abr1, &abr2}

		n := nodes[0]
		for _, abr := range abrs {
			bits, err := abr.Marshal()
			require.Nil(t, err)

			n.pubsub.Publish(transactionTopic, bits)
		}
		waitForAllAbrs(t, ctx, nodes, abrs)

		current := n.rounds.Current()
		prev, _ := n.rounds.Get(current.height - 1)
		// make sure the round was only decided with a single ABR
		require.Equal(t, 1, prev.snowball.Preferred().Checkpoint.Length())
		// and that the mempool is now empty
		require.Equal(t, 0, n.mempool.Length())
	})
}

func abrToHamtCID(ctx context.Context, abr *services.AddBlockRequest) cid.Cid {
	store := nodestore.MustMemoryStore(ctx)
	hamtStore := hamtwrapper.DagStoreToCborIpld(store)
	id, _ := hamtStore.Put(ctx, abr)
	return id
}
