package gossip4

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	logging "github.com/ipfs/go-log"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
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
		p2pNode, peer, err := p2p.NewHostAndBitSwapPeer(ctx) // TODO: options?
		if err != nil {
			return nil, nil, fmt.Errorf("error making node: %v", err)
		}

		n, err := NewNode(ctx, &NewNodeOptions{
			P2PNode:          p2pNode,
			SignKey:          testSet.SignKeys[i],
			NotaryGroup:      ng,
			DagStore:         peer,
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
	// logging.SetLogLevel("pubsub", "debug")
	testLogger := logging.Logger("TestEndToEnd")
	logging.SetLogLevel("TestEndToEnd", "INFO")

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		testLogger.Debugf("test finished")
		cancel()
	}()

	numMembers := 7
	ts := testnotarygroup.NewTestSet(t, numMembers)
	ng, nodes, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)
	require.Len(t, nodes, numMembers)
	fmt.Println("quorum count: ", ng.QuorumCount())

	n := nodes[0]
	fmt.Println("signerIndex: ", n.signerIndex)
	// logging.SetLogLevel(fmt.Sprintf("node-%d", n.signerIndex), "info")
	bootAddrs := testnotarygroup.BootstrapAddresses(n.p2pNode)

	for i, node := range nodes {
		logging.SetLogLevel(fmt.Sprintf("node-%d", node.signerIndex), "info")

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

	cl, err := n.p2pNode.Bootstrap(testnotarygroup.BootstrapAddresses(nodes[1].p2pNode))
	require.Nil(t, err)
	defer cl.Close()
	err = n.p2pNode.WaitForBootstrap(1, 2*time.Second)
	require.Nil(t, err)

	transCount := difficulty * 4 // four times necessary
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
		err = n.pubsub.Publish(transactionTopic, bits)
		require.Nil(t, err)
	}

	allIncluded := func() bool {
		if n.latestCheckpoint == nil || n.latestCheckpoint.node == nil {
			return false
		}
		for i, tx := range trans {
			did := string(tx.ObjectId)
			var tip cid.Cid
			err := n.latestCheckpoint.node.Find(ctx, did, &tip)
			if err == hamt.ErrNotFound {
				// then check if it's in the inprogress
				err := n.inprogressCheckpoint.node.Find(ctx, did, &tip)
				if err == hamt.ErrNotFound {
					testLogger.Debugf("couldn't find: %d", i)
					return false
				}
				if err == nil {
					continue
				}
			}
			require.Nil(t, err)
		}
		return true
	}

	// wait for all transCount transactions to be included in the currentCommit
	timer := time.NewTimer(5 * time.Second)
looper:
	for {
		select {
		case <-timer.C:
			testLogger.Debugf("failing on timeout")
			t.Fatalf("timeout waiting for all transactions")
		default:
			// do nothing
		}
		if allIncluded() {
			testLogger.Debugf("found all transactions at height %d", n.inprogressCheckpoint.Height)
			break looper
		}

		testLogger.Debugf("inprogress height %d", n.inprogressCheckpoint.Height)
		time.Sleep(100 * time.Millisecond)
	}
	timer.Stop()

	// there was a weird syncronization bug where if you just do n.inProgressCheckpoint you might end up with a different
	// height than what is in the actor. Requesting the checkpoint from the actor lets us bypass any locks, etc but still
	// get the acutal height
	fut := actor.EmptyRootContext.RequestFuture(n.pid, &getInProgressCheckpoint{}, 1*time.Second)
	res, err := fut.Result()
	require.Nil(t, err)

	assert.Truef(t, res.(*Checkpoint).Height >= 1, "in progress checkpoint %d was not higher than 1", res.(*Checkpoint).Height)
}
