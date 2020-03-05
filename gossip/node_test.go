package gossip

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-bitswap"
	msgio "github.com/libp2p/go-msgio"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/hamtwrapper"

	"github.com/ipfs/go-cid"

	"github.com/stretchr/testify/require"

	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"

	"github.com/quorumcontrol/tupelo-go-sdk/gossip/testhelpers"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/types"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
)

func newTupeloSystem(ctx context.Context, t testing.TB, testSet *testnotarygroup.TestSet) (*types.NotaryGroup, []*Node, error) {
	name := t.Name()
	nodes := make([]*Node, len(testSet.SignKeys))

	ng := types.NewNotaryGroup("testnotary")
	for i, signKey := range testSet.SignKeys {
		sk := signKey
		signer := types.NewRemoteSigner(testSet.PubKeys[i], sk.MustVerKey())
		ng.AddSigner(signer)
	}

	for i := range ng.AllSigners() {
		p2pNode, peer, err := p2p.NewHostAndBitSwapPeer(ctx, p2p.WithKey(testSet.EcdsaKeys[i]), p2p.WithBitswapOptions(bitswap.ProvideEnabled(false)))
		if err != nil {
			return nil, nil, fmt.Errorf("error making node: %v", err)
		}

		n, err := NewNode(ctx, &NewNodeOptions{
			P2PNode:     p2pNode,
			SignKey:     testSet.SignKeys[i],
			NotaryGroup: ng,
			DagStore:    peer,
			Name:        strconv.Itoa(i) + "-" + name,
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
		// logging.SetLogLevel(fmt.Sprintf("node-%d", i), "debug")

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
			r, _ := n.rounds.Get(current.height - 1)
			abr, err := r.state.Find(ctx, did)
			require.Nil(t, err)
			if abr == nil {
				return false
			}
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
	_, nodes, err := newTupeloSystem(ctx, t, ts)
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
	group, nodes, err := newTupeloSystem(ctx, t, ts)
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

	sub, err := nodes[0].pubsub.Subscribe(group.ID)
	require.Nil(t, err)

	for i, abr := range abrs {
		bits, err := abr.Marshal()
		require.Nil(t, err)

		err = nodes[i%(len(nodes)-1)].pubsub.Publish(group.Config().TransactionTopic, bits)
		require.Nil(t, err)
	}

	waitForAllAbrs(t, ctx, nodes, abrs)

	// test that it receives the round confirmation signatures
	for range nodes {
		_, err = sub.Next(ctx)
		require.Nil(t, err)
	}

}

func TestIdleRoundsRepublish(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	numMembers := 3
	ts := testnotarygroup.NewTestSet(t, numMembers)
	group, nodes, err := newTupeloSystem(ctx, t, ts)
	require.Nil(t, err)
	require.Len(t, nodes, numMembers)

	startNodes(t, ctx, nodes)

	testStore := hamtwrapper.DagStoreToCborIpld(nodestore.MustMemoryStore(ctx))
	abr := testhelpers.NewValidTransaction(t)
	id, err := testStore.Put(ctx, abr)
	require.Nil(t, err)
	t.Logf("transaction as cid %s", id.String())

	sub, err := nodes[0].pubsub.Subscribe(group.ID)
	require.Nil(t, err)

	bits, err := abr.Marshal()
	require.Nil(t, err)

	err = nodes[0].pubsub.Publish(group.Config().TransactionTopic, bits)
	require.Nil(t, err)

	waitForAllAbrs(t, ctx, nodes, []*services.AddBlockRequest{&abr})

	// test that it receives the round confirmation signatures
	for range nodes {
		_, err = sub.Next(ctx)
		require.Nil(t, err)
	}
	// and now we should get them again while idle
	for range nodes {
		_, err = sub.Next(ctx)
		require.Nil(t, err)
	}
	// and they should happen again
	for range nodes {
		_, err = sub.Next(ctx)
		require.Nil(t, err)
	}
}

func TestByzantineCases(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	numMembers := 3
	ts := testnotarygroup.NewTestSet(t, numMembers)
	group, nodes, err := newTupeloSystem(ctx, t, ts)
	require.Nil(t, err)
	require.Len(t, nodes, numMembers)

	startNodes(t, ctx, nodes)

	t.Run("asks for a snowball with no Txs", func(t *testing.T) {
		abr := testhelpers.NewValidTransaction(t)
		wrapper := &AddBlockWrapper{
			AddBlockRequest: &abr,
		}
		wrapper.StartTrace("gossip4.transaction")

		actor.EmptyRootContext.Send(nodes[0].PID(), wrapper)

		waitForAllAbrs(t, ctx, nodes, []*services.AddBlockRequest{&abr})
		// ok so we have round 0 decided let's ask for round 1 but no one has any transactions

		for _, node := range nodes {
			otherSigner := group.AllSigners()[(node.signerIndex+1)%(len(nodes)-1)]
			node.p2pNode.WaitForBootstrap(2, 10*time.Second)

			s, err := node.p2pNode.NewStream(ctx, otherSigner.DstKey, gossipProtocol)
			require.Nil(t, err)
			defer s.Close()

			sw := &safewrap.SafeWrap{}
			wrapped := sw.WrapObject(node.rounds.Current().height + 1)

			writer := msgio.NewVarintWriter(s)
			err = writer.WriteMsg(wrapped.RawData())
			require.Nil(t, err)
		}

		// now wait a bit
		time.Sleep(1 * time.Second)

		// and we should still be able to send in a transaction and get a round
		abr2 := testhelpers.NewValidTransaction(t)
		wrapper2 := &AddBlockWrapper{
			AddBlockRequest: &abr2,
		}
		wrapper2.StartTrace("gossip4.transaction")

		actor.EmptyRootContext.Send(nodes[1].PID(), wrapper2)

		waitForAllAbrs(t, ctx, nodes, []*services.AddBlockRequest{&abr2})
	})

	t.Run("different transactions at each node", func(t *testing.T) {
		abrs := make([]*services.AddBlockRequest, numMembers)
		for i := 0; i < numMembers; i++ {
			abr := testhelpers.NewValidTransaction(t)
			abrs[i] = &abr
		}

		for i, abr := range abrs {
			wrapper := &AddBlockWrapper{
				AddBlockRequest: abr,
			}
			wrapper.StartTrace("gossip3.transaction")

			actor.EmptyRootContext.Send(nodes[i].PID(), wrapper)
		}
		waitForAllAbrs(t, ctx, nodes, abrs)
	})

	t.Run("one node starts with all the transactions", func(t *testing.T) {
		abrs := make([]*services.AddBlockRequest, numMembers)
		for i := 0; i < numMembers; i++ {
			abr := testhelpers.NewValidTransaction(t)
			abrs[i] = &abr
		}

		for _, abr := range abrs {
			wrapper := &AddBlockWrapper{
				AddBlockRequest: abr,
			}
			wrapper.StartTrace("gossip3.transaction")

			actor.EmptyRootContext.Send(nodes[0].PID(), wrapper)
		}
		waitForAllAbrs(t, ctx, nodes, abrs)
	})

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

			err = n.pubsub.Publish(group.Config().TransactionTopic, bits)
			require.Nil(t, err)
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
