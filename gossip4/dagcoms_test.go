package gossip4

import (
	"context"
	"fmt"
	"testing"
	"time"

	logging "github.com/ipfs/go-log"

	dsync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/require"

	"github.com/ipfs/go-datastore"

	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"

	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/testhelpers"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
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
		p2pNode, peer, err := p2p.NewHostAndBitSwapPeer(ctx) // TODO: options?
		if err != nil {
			return nil, nil, fmt.Errorf("error making node: %v", err)
		}

		kvStore := dsync.MutexWrap(datastore.NewMapDatastore())

		n, err := NewNode(ctx, &NewNodeOptions{
			P2PNode:          p2pNode,
			SignKey:          testSet.SignKeys[i],
			NotaryGroup:      ng,
			DagStore:         peer,
			KVStore:          kvStore,
			latestCheckpoint: cid.Undef,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("error making node: %v", err)
		}
		nodes[i] = n
	}

	return ng, nodes, nil
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

	n := nodes[0]
	bootAddrs := testnotarygroup.BootstrapAddresses(n.p2pNode)

	for i, node := range nodes {
		logging.SetLogLevel(fmt.Sprintf("node-%d", i), "debug")

		if i > 0 {
			cl, err := node.p2pNode.Bootstrap(bootAddrs)
			require.Nil(t, err)
			defer cl.Close()

			err = node.p2pNode.WaitForBootstrap(1, 2*time.Second)
			require.Nil(t, err)
		}
		err = node.Start(ctx)
		require.Nil(t, err)
	}

	trans := testhelpers.NewValidTransaction(t)

	bits, err := trans.Marshal()
	require.Nil(t, err)

	err = n.pubsub.Publish(transactionTopic, bits)
	require.Nil(t, err)
	time.Sleep(2 * time.Second)
}
