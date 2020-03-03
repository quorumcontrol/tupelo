package benchmark

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ipfs/go-bitswap"
	logging "github.com/ipfs/go-log"

	"github.com/quorumcontrol/tupelo-go-sdk/gossip/client"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/client/pubsubinterfaces/pubsubwrapper"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/types"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
	"github.com/quorumcontrol/tupelo/gossip"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/require"
)

func newTupeloSystem(ctx context.Context, t testing.TB, testSet *testnotarygroup.TestSet) (*types.NotaryGroup, []*gossip.Node) {
	nodes := make([]*gossip.Node, len(testSet.SignKeys))

	ng := types.NewNotaryGroup("testnotary")
	for i, signKey := range testSet.SignKeys {
		sk := signKey
		signer := types.NewRemoteSigner(testSet.PubKeys[i], sk.MustVerKey())
		ng.AddSigner(signer)
	}

	for i := range ng.AllSigners() {
		p2pNode, peer, err := p2p.NewHostAndBitSwapPeer(ctx, p2p.WithKey(testSet.EcdsaKeys[i]), p2p.WithBitswapOptions(bitswap.ProvideEnabled(false)))
		require.Nil(t, err)

		n, err := gossip.NewNode(ctx, &gossip.NewNodeOptions{
			P2PNode:     p2pNode,
			SignKey:     testSet.SignKeys[i],
			NotaryGroup: ng,
			DagStore:    peer,
		})
		require.Nil(t, err)
		nodes[i] = n
	}
	// setting log level to debug because it's useful output on test failures
	// this happens after the AllSigners loop because the node name is based on the
	// index in the signers
	for i := range ng.AllSigners() {
		err := logging.SetLogLevel(fmt.Sprintf("node-%d", i), "debug")
		require.Nil(t, err)
	}

	return ng, nodes
}

func startNodes(t *testing.T, ctx context.Context, nodes []*gossip.Node, bootAddrs []string) {
	for _, node := range nodes {
		err := node.Bootstrap(ctx, bootAddrs)
		require.Nil(t, err)
		err = node.Start(ctx)
		require.Nil(t, err)
	}
}

func newClient(ctx context.Context, group *types.NotaryGroup, bootAddrs []string) (*client.Client, error) {
	cliHost, peer, err := p2p.NewHostAndBitSwapPeer(ctx)
	if err != nil {
		return nil, err
	}

	_, err = cliHost.Bootstrap(bootAddrs)
	if err != nil {
		return nil, err
	}

	err = cliHost.WaitForBootstrap(len(group.AllSigners()), 5*time.Second)
	if err != nil {
		return nil, err
	}

	cli := client.New(group, pubsubwrapper.WrapLibp2p(cliHost.GetPubSub()), peer)

	err = cli.Start(ctx)
	if err != nil {
		return nil, err
	}
	return cli, nil
}

func TestBenchmarker(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	numMembers := 3
	ts := testnotarygroup.NewTestSet(t, numMembers)
	group, nodes := newTupeloSystem(ctx, t, ts)
	require.Len(t, nodes, numMembers)

	booter, err := p2p.NewHostFromOptions(ctx)
	require.Nil(t, err)

	bootAddrs := make([]string, len(booter.Addresses()))
	for i, addr := range booter.Addresses() {
		bootAddrs[i] = addr.String()
	}

	startNodes(t, ctx, nodes, bootAddrs)

	cli, err := newClient(ctx, group, bootAddrs)
	require.Nil(t, err)

	ben := NewBenchmark(cli, 3, 3*time.Second, 1*time.Second)
	require.NotNil(t, ben)

	res := ben.Run(ctx)
	require.InDelta(t, int64(9), res.Total, float64(1))
	require.InDelta(t, 9, res.Measured, float64(1))
}
