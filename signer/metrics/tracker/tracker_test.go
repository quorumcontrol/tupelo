package tracker

import (
	"context"
	"testing"
	"time"

	"github.com/ipfs/go-bitswap"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"

	// logging "github.com/ipfs/go-log"

	"github.com/stretchr/testify/require"

	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo/sdk/gossip/client/pubsubinterfaces/pubsubwrapper"
	"github.com/quorumcontrol/tupelo/sdk/p2p"

	"github.com/quorumcontrol/tupelo/sdk/gossip/client"
	"github.com/quorumcontrol/tupelo/sdk/gossip/testhelpers"
	"github.com/quorumcontrol/tupelo/sdk/gossip/types"

	"github.com/quorumcontrol/tupelo/signer/gossip"
	"github.com/quorumcontrol/tupelo/signer/metrics/classifier"
	"github.com/quorumcontrol/tupelo/signer/metrics/result"
	"github.com/quorumcontrol/tupelo/signer/testnotarygroup"
)

func TestTracker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	numMembers := 3
	ts := testnotarygroup.NewTestSet(t, numMembers)
	group, nodes, bootAddrs := newTupeloSystem(ctx, t, ts)
	require.Len(t, nodes, numMembers)

	cliNode, dagStore, err := p2p.NewHostAndBitSwapPeer(ctx, p2p.WithBitswapOptions(bitswap.ProvideEnabled(true)))
	require.Nil(t, err)
	_, err = cliNode.Bootstrap(bootAddrs)
	require.Nil(t, err)

	err = cliNode.WaitForBootstrap(len(nodes), 10*time.Second)
	require.Nil(t, err)
	cli := client.New(group, pubsubwrapper.WrapLibp2p(cliNode.GetPubSub()), dagStore)

	err = cli.Start(ctx)
	require.Nil(t, err)

	recorder := &testRecorder{}

	// logging.SetLogLevel("g4-client", "DEBUG")

	sw := &safewrap.SafeWrap{}

	abrCount := 5
	abrs := make([]*services.AddBlockRequest, abrCount)
	allDids := []string{}
	var lastRoundCid cid.Cid
	var roundHeight uint64
	for i := 0; i < abrCount; i++ {
		abr := testhelpers.NewValidTransaction(t)
		for _, node := range abr.State {
			err = dagStore.Add(ctx, sw.Decode(node))
			require.Nil(t, err)
			require.Nil(t, sw.Err)
		}
		abrs[i] = &abr
		proof, err := cli.Send(ctx, &abr, 10*time.Second)
		require.Nil(t, err)
		allDids = append(allDids, string(abr.GetObjectId()))

		if proof.GetRound().Height >= roundHeight {
			lastRoundCid, err = cid.Cast(proof.GetRoundConfirmation().GetRoundCid())
			require.Nil(t, err)
			roundHeight = proof.GetRound().Height
		}
	}
	require.NotEmpty(t, lastRoundCid)

	withFirstClassifier := func(ctx context.Context, startDag *dag.Dag, endDag *dag.Dag) (string, map[string]interface{}, error) {
		dclass, dtags, err := classifier.Default(ctx, startDag, endDag)
		if err != nil {
			return dclass, dtags, err
		}

		root := &chaintree.RootNode{}
		err = endDag.ResolveInto(ctx, []string{}, root)
		if err != nil {
			return dclass, dtags, err
		}
		if root.Id == allDids[0] {
			dclass = "first"
		}
		return dclass, dtags, err
	}

	tracker := New(&Options{
		Tupelo:     cli,
		Nodestore:  dagStore,
		Classifier: withFirstClassifier,
		Recorder:   recorder,
	})

	err = tracker.TrackAll(ctx)
	require.Nil(t, err)

	for _, did := range allDids {
		_, ok := recorder.results[did]
		require.True(t, ok)
	}

	res, ok := recorder.results[allDids[0]]
	require.True(t, ok)
	require.Equal(t, res.DeltaBlocks, uint64(1))
	require.Equal(t, res.TotalBlocks, uint64(1))
	require.Equal(t, res.Did, allDids[0])
	require.Equal(t, res.Round, roundHeight)
	require.Equal(t, res.Tags["new"], true)

	defaultSum := recorder.summaries["default"]
	require.Equal(t, defaultSum.Type, "default")
	require.Equal(t, defaultSum.Count, uint64(4))
	require.Equal(t, defaultSum.DeltaCount, uint64(4))
	require.Equal(t, defaultSum.DeltaBlocks, uint64(4))
	require.Equal(t, defaultSum.TotalBlocks, uint64(4))
	require.Equal(t, defaultSum.Round, roundHeight)
	require.Equal(t, defaultSum.RoundCid, lastRoundCid.String())
	require.Equal(t, defaultSum.LastRound, uint64(0))
	require.Equal(t, defaultSum.LastRoundCid, "")

	firstSum := recorder.summaries["first"]
	require.Equal(t, firstSum.Type, "first")
	require.Equal(t, firstSum.Count, uint64(1))
	require.Equal(t, firstSum.DeltaCount, uint64(1))
	require.Equal(t, firstSum.DeltaBlocks, uint64(1))
	require.Equal(t, firstSum.TotalBlocks, uint64(1))
	require.Equal(t, firstSum.Round, roundHeight)
	require.Equal(t, firstSum.RoundCid, lastRoundCid.String())
	require.Equal(t, firstSum.LastRound, uint64(0))
	require.Equal(t, firstSum.LastRoundCid, "")

	var singleTreeRoundCid cid.Cid

	for i := 0; i < abrCount; i++ {
		abr := testhelpers.NewValidTransaction(t)
		for _, node := range abr.State {
			err = dagStore.Add(ctx, sw.Decode(node))
			require.Nil(t, err)
			require.Nil(t, sw.Err)
		}
		abrs[i] = &abr
		proof, err := cli.Send(ctx, &abr, 10*time.Second)
		require.Nil(t, err)

		allDids = append(allDids, string(abr.GetObjectId()))

		if i == 0 {
			singleTreeRoundCid, err = cid.Cast(proof.GetRoundConfirmation().GetRoundCid())
			require.Nil(t, err)
		}
	}

	err = tracker.TrackBetween(ctx, lastRoundCid, singleTreeRoundCid)
	require.Nil(t, err)
	require.Len(t, recorder.results, 6)

	res, ok = recorder.results[allDids[0]]
	require.True(t, ok)
	require.Equal(t, res.DeltaBlocks, uint64(0))
	require.Equal(t, res.TotalBlocks, uint64(1))
	require.Equal(t, res.Did, allDids[0])
	require.Equal(t, res.Round, roundHeight+1)
	_, newTag := res.Tags["new"]
	require.False(t, newTag)

	defaultSum = recorder.summaries["default"]
	require.Equal(t, defaultSum.Type, "default")
	require.Equal(t, defaultSum.Count, uint64(5))
	require.Equal(t, defaultSum.DeltaCount, uint64(1))
	require.Equal(t, defaultSum.DeltaBlocks, uint64(1))
	require.Equal(t, defaultSum.TotalBlocks, uint64(5))
	require.Equal(t, defaultSum.Round, roundHeight+1)
	require.Equal(t, defaultSum.LastRound, roundHeight)
	require.Equal(t, defaultSum.LastRoundCid, lastRoundCid.String())

	firstSum = recorder.summaries["first"]
	require.Equal(t, firstSum.Type, "first")
	require.Equal(t, firstSum.Count, uint64(1))
	require.Equal(t, firstSum.DeltaCount, uint64(0))
	require.Equal(t, firstSum.DeltaBlocks, uint64(0))
	require.Equal(t, firstSum.TotalBlocks, uint64(1))
	require.Equal(t, firstSum.Round, roundHeight+1)
	require.Equal(t, firstSum.LastRound, roundHeight)
	require.Equal(t, firstSum.LastRoundCid, lastRoundCid.String())

	err = tracker.TrackFrom(ctx, lastRoundCid)
	require.Nil(t, err)
	require.Len(t, recorder.results, 10)

	for _, did := range allDids {
		_, ok := recorder.results[did]
		require.True(t, ok)
	}

	defaultSum = recorder.summaries["default"]
	require.Equal(t, defaultSum.Type, "default")
	require.Equal(t, defaultSum.Count, uint64(9))
	require.Equal(t, defaultSum.DeltaCount, uint64(5))
	require.Equal(t, defaultSum.DeltaBlocks, uint64(5))
	require.Equal(t, defaultSum.TotalBlocks, uint64(9))
	require.Equal(t, defaultSum.Round, roundHeight+5)
	require.Equal(t, defaultSum.LastRound, roundHeight)
	require.Equal(t, defaultSum.LastRoundCid, lastRoundCid.String())

	firstSum = recorder.summaries["first"]
	require.Equal(t, firstSum.Type, "first")
	require.Equal(t, firstSum.Count, uint64(1))
	require.Equal(t, firstSum.DeltaCount, uint64(0))
	require.Equal(t, firstSum.DeltaBlocks, uint64(0))
	require.Equal(t, firstSum.TotalBlocks, uint64(1))
	require.Equal(t, defaultSum.Round, roundHeight+5)
	require.Equal(t, defaultSum.LastRound, roundHeight)
	require.Equal(t, defaultSum.LastRoundCid, lastRoundCid.String())

}

type testRecorder struct {
	results   map[string]*result.Result
	summaries map[string]*result.Summary
}

func (r *testRecorder) Start(ctx context.Context, startRound *types.RoundWrapper, endRound *types.RoundWrapper) error {
	r.results = make(map[string]*result.Result)
	r.summaries = make(map[string]*result.Summary)
	return nil
}

func (r *testRecorder) Record(ctx context.Context, res *result.Result) error {
	r.results[res.Did] = res
	return nil
}

func (r *testRecorder) Finish(ctx context.Context, summaries []*result.Summary) error {
	for _, s := range summaries {
		r.summaries[s.Type] = s
	}
	return nil
}

func newTupeloSystem(ctx context.Context, t testing.TB, testSet *testnotarygroup.TestSet) (*types.NotaryGroup, []*gossip.Node, []string) {
	nodes := make([]*gossip.Node, len(testSet.SignKeys))

	ng := types.NewNotaryGroup("testnotary")
	for i, signKey := range testSet.SignKeys {
		sk := signKey
		signer := types.NewRemoteSigner(testSet.PubKeys[i], sk.MustVerKey())
		ng.AddSigner(signer)
	}

	bootAddrs := make([]string, 0)

	for i := range ng.AllSigners() {
		p2pNode, peer, err := p2p.NewHostAndBitSwapPeer(ctx, p2p.WithKey(testSet.EcdsaKeys[i]), p2p.WithBitswapOptions(bitswap.ProvideEnabled(true)))
		require.Nil(t, err)

		dataStore := dsync.MutexWrap(datastore.NewMapDatastore())

		n, err := gossip.NewNode(ctx, &gossip.NewNodeOptions{
			P2PNode:     p2pNode,
			SignKey:     testSet.SignKeys[i],
			NotaryGroup: ng,
			DagStore:    peer,
			Datastore:   dataStore,
		})
		require.Nil(t, err)
		nodes[i] = n

		// logging.SetLogLevel(fmt.Sprintf("node-%d", i), "DEBUG")

		if i > 0 {
			err := n.Bootstrap(ctx, bootAddrs)
			require.Nil(t, err)
		}

		for _, addr := range p2pNode.Addresses() {
			bootAddrs = append(bootAddrs, addr.String())
		}

		err = n.Start(ctx)
		require.Nil(t, err)
	}

	return ng, nodes, bootAddrs
}
