package gossip4

import (
	"context"
	"fmt"
	"testing"
	"time"

	logging "github.com/ipfs/go-log"

	"github.com/stretchr/testify/require"

	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/messages/v2/build/go/services"
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
		p2pNode, peer, err := p2p.NewHostAndBitSwapPeer(ctx, p2p.WithKey(testSet.EcdsaKeys[i])) // TODO: options?
		if err != nil {
			return nil, nil, fmt.Errorf("error making node: %v", err)
		}

		n, err := NewNode(ctx, &NewNodeOptions{
			P2PNode:      p2pNode,
			SignKey:      testSet.SignKeys[i],
			NotaryGroup:  ng,
			DagStore:     peer,
			CurrentRound: 0,
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
	// logging.SetLogLevel("pubsub", "debug")
	testLogger := logging.Logger("TestEndToEnd")
	logging.SetLogLevel("TestEndToEnd", "INFO")
	logging.SetLogLevel("snowball", "INFO")

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		testLogger.Debugf("test finished")
		cancel()
	}()

	bootstrapper, err := p2p.NewHostFromOptions(ctx)
	require.Nil(t, err)

	numMembers := 7
	ts := testnotarygroup.NewTestSet(t, numMembers)
	ng, nodes, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)
	require.Len(t, nodes, numMembers)
	fmt.Println("quorum count: ", ng.QuorumCount())

	n := nodes[0]
	fmt.Println("signerIndex: ", n.signerIndex)
	// logging.SetLogLevel(fmt.Sprintf("node-%d", n.signerIndex), "info")
	bootAddrs := testnotarygroup.BootstrapAddresses(bootstrapper)

	for i, node := range nodes {
		logging.SetLogLevel(fmt.Sprintf("node-%d", node.signerIndex), "INFO")

		if i > 0 {
			cl, err := node.p2pNode.Bootstrap(bootAddrs)
			require.Nil(t, err)
			defer cl.Close()

			err = node.p2pNode.WaitForBootstrap(1, 2*time.Second)
			require.Nil(t, err)
			err = node.p2pNode.(*p2p.LibP2PHost).StartDiscovery("gossip4")
			require.Nil(t, err)
		}
		err = node.Start(ctx)
		require.Nil(t, err)
	}

	cl, err := n.p2pNode.Bootstrap(bootAddrs)
	require.Nil(t, err)
	defer cl.Close()
	err = n.p2pNode.WaitForBootstrap(len(nodes)-1, 2*time.Second)
	require.Nil(t, err)
	err = n.p2pNode.(*p2p.LibP2PHost).StartDiscovery("gossip4")
	require.Nil(t, err)

	// n.p2pNode.(*p2p.LibP2PHost).WaitForDiscovery("gossip4", 1, 10*time.Second)

	transCount := 20
	trans := make([]*services.AddBlockRequest, transCount)

	testStore := dagStoreToCborIpld(nodestore.MustMemoryStore(ctx))

	for i := 0; i < transCount; i++ {
		tran := testhelpers.NewValidTransaction(t)

		// for examining the log only:
		id, err := testStore.Put(ctx, tran)
		require.Nil(t, err)
		testLogger.Infof("transaction %d has cid %s", i, id.String())

		trans[i] = &tran
	}

	for i, trans := range trans {
		bits, err := trans.Marshal()
		require.Nil(t, err)

		testLogger.Debugf("sending %d (%s)", i, string(trans.ObjectId))
		err = bootstrapper.GetPubSub().Publish(transactionTopic, bits)
		require.Nil(t, err)
	}

	time.Sleep(2 * time.Second)
}
