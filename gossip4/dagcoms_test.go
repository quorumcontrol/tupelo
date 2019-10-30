package gossip4

import (
	"context"
	"fmt"
	"testing"

	dsync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/require"

	"github.com/ipfs/go-datastore"

	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"

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

		p2pNode, peer, err := p2p.NewHostAndBitSwapPeer(ctx) // TODO: options?
		if err != nil {
			return nil, nil, fmt.Errorf("error making node: %v", err)
		}

		kvStore := dsync.MutexWrap(datastore.NewMapDatastore())

		n, err := NewNode(ctx, &NewNodeOptions{
			P2PNode:          p2pNode,
			SignKey:          sk,
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
}
