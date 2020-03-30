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

	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/client/pubsubinterfaces/pubsubwrapper"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"

	"github.com/quorumcontrol/tupelo-go-sdk/gossip/client"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/testhelpers"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/types"

	"github.com/quorumcontrol/tupelo/gossip"
	"github.com/quorumcontrol/tupelo/metrics/classifier"
	"github.com/quorumcontrol/tupelo/metrics/result"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
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
	tracker := New(&Options{
		Tupelo:     cli,
		Nodestore:  dagStore,
		Classifier: classifier.Default,
		Recorder:   recorder,
	})

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

	recorder.results = make(map[string]*result.Result)
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

	recorder.results = make(map[string]*result.Result)
	err = tracker.TrackFrom(ctx, lastRoundCid)
	require.Nil(t, err)
	require.Len(t, recorder.results, 10)

	for _, did := range allDids {
		_, ok := recorder.results[did]
		require.True(t, ok)
	}
}

type testRecorder struct {
	results map[string]*result.Result
}

func (r *testRecorder) Start(ctx context.Context, startRound *types.RoundWrapper, endRound *types.RoundWrapper) error {
	return nil
}

func (r *testRecorder) Record(ctx context.Context, res *result.Result) error {
	if r.results == nil {
		r.results = make(map[string]*result.Result)
	}
	r.results[res.Did] = res
	return nil
}

func (r *testRecorder) Finish(ctx context.Context) error {
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
