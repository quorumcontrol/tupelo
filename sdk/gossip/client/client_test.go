package client

import (
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"

	"github.com/ethereum/go-ethereum/crypto"
	ds "github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"

	"github.com/quorumcontrol/tupelo/sdk/gossip/testhelpers"

	"github.com/quorumcontrol/messages/v2/build/go/gossip"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/messages/v2/build/go/transactions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tupelogossip "github.com/quorumcontrol/tupelo/signer/gossip"

	"github.com/quorumcontrol/tupelo/sdk/consensus"
	"github.com/quorumcontrol/tupelo/sdk/gossip/types"
	"github.com/quorumcontrol/tupelo/sdk/p2p"
	"github.com/quorumcontrol/tupelo/sdk/testnotarygroup"
)

const groupMembers = 3

func init() {
	// round heartbeat should be fast in tests with no latency
	DefaultRoundWaitTimeout = 1 * time.Second
}

func newTupeloSystem(ctx context.Context, testSet *testnotarygroup.TestSet) (*types.NotaryGroup, []*tupelogossip.Node, error) {
	nodes := make([]*tupelogossip.Node, len(testSet.SignKeys))

	ng := types.NewNotaryGroup("testnotary")

	for i, signKey := range testSet.SignKeys {
		sk := signKey
		signer := types.NewRemoteSigner(testSet.PubKeys[i], sk.MustVerKey())
		ng.AddSigner(signer)
	}

	for i := range ng.AllSigners() {
		p2pNode, peer, err := p2p.NewHostAndBitSwapPeer(ctx, p2p.WithKey(testSet.EcdsaKeys[i]))
		if err != nil {
			return nil, nil, fmt.Errorf("error making node: %v", err)
		}

		n, err := tupelogossip.NewNode(ctx, &tupelogossip.NewNodeOptions{
			P2PNode:     p2pNode,
			SignKey:     testSet.SignKeys[i],
			NotaryGroup: ng,
			Datastore:   dsync.MutexWrap(ds.NewMapDatastore()),
			DagStore:    peer,
		})
		if err != nil {
			return nil, nil, fmt.Errorf("error making node: %v", err)
		}
		nodes[i] = n
	}
	// setting log level to debug because it's useful output on test failures
	// this happens after the AllSigners loop because the node name is based on the
	// index in the signers
	for i := range ng.AllSigners() {
		if err := logging.SetLogLevel(fmt.Sprintf("node-%d", i), "debug"); err != nil {
			return nil, nil, fmt.Errorf("error setting log level: %v", err)
		}
	}

	return ng, nodes, nil
}

func startNodes(t *testing.T, ctx context.Context, nodes []*tupelogossip.Node, bootAddrs []string) {
	for _, node := range nodes {
		err := node.Bootstrap(ctx, bootAddrs)
		require.Nil(t, err)
		err = node.Start(ctx)
		require.Nil(t, err)
	}
}

func startRounds(t *testing.T, parentCtx context.Context, group *types.NotaryGroup, bootAddrs []string) {
	ctx, cancel := context.WithCancel(parentCtx)
	defer cancel()

	cli, err := newClient(ctx, group, bootAddrs)
	require.Nil(t, err)

	treeKey, err := crypto.GenerateKey()
	require.Nil(t, err)
	tree, err := consensus.NewSignedChainTree(ctx, treeKey.PublicKey, nodestore.MustMemoryStore(ctx))
	require.Nil(t, err)

	txn, err := chaintree.NewSetDataTransaction("just-a-round-igniter", "success")
	require.Nil(t, err)

	proof, err := cli.PlayTransactions(ctx, tree, treeKey, []*transactions.Transaction{txn})
	require.Nil(t, err)
	assert.Equal(t, proof.Tip, tree.Tip().Bytes())
}

func newClient(ctx context.Context, group *types.NotaryGroup, bootAddrs []string) (*Client, error) {
	group.Config().BootstrapAddresses = bootAddrs
	cli := New(WithNotaryGroup(group))

	err := logging.SetLogLevel("g4-client", "debug")
	if err != nil {
		return nil, err
	}

	err = cli.Start(ctx)
	if err != nil {
		return nil, err
	}

	return cli, nil
}

func TestClientSendTransactions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ts := testnotarygroup.NewTestSet(t, groupMembers)
	group, nodes, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)
	require.Len(t, nodes, groupMembers)

	booter, err := p2p.NewHostFromOptions(ctx)
	require.Nil(t, err)

	bootAddrs := make([]string, len(booter.Addresses()))
	for i, addr := range booter.Addresses() {
		bootAddrs[i] = addr.String()
	}

	startNodes(t, ctx, nodes, bootAddrs)

	t.Run("test basic setup", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		cli, err := newClient(ctx, group, bootAddrs)
		require.Nil(t, err)

		treeKey, err := crypto.GenerateKey()
		require.Nil(t, err)
		tree, err := consensus.NewSignedChainTree(ctx, treeKey.PublicKey, nodestore.MustMemoryStore(ctx))
		require.Nil(t, err)

		txn, err := chaintree.NewSetDataTransaction("down/in/the/thing", "sometestvalue")
		require.Nil(t, err)

		proof, err := cli.PlayTransactions(ctx, tree, treeKey, []*transactions.Transaction{txn})
		require.Nil(t, err)
		assert.Equal(t, proof.Tip, tree.Tip().Bytes())
	})

	t.Run("test 3 subsequent transactions", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		cli, err := newClient(ctx, group, bootAddrs)
		require.Nil(t, err)

		treeKey, err := crypto.GenerateKey()
		require.Nil(t, err)
		tree, err := consensus.NewSignedChainTree(ctx, treeKey.PublicKey, nodestore.MustMemoryStore(ctx))
		require.Nil(t, err)

		txn, err := chaintree.NewSetDataTransaction("down/in/the/thing", "sometestvalue")
		require.Nil(t, err)

		proof, err := cli.PlayTransactions(ctx, tree, treeKey, []*transactions.Transaction{txn})
		require.Nil(t, err)
		assert.Equal(t, proof.Tip, tree.Tip().Bytes())

		txn2, err := chaintree.NewSetDataTransaction("down/in/the/thing", "some other2")
		require.Nil(t, err)

		proof2, err := cli.PlayTransactions(ctx, tree, treeKey, []*transactions.Transaction{txn2})
		require.Nil(t, err)
		assert.Equal(t, proof2.Tip, tree.Tip().Bytes())

		txn3, err := chaintree.NewSetDataTransaction("down/in/the/thing", "some other3")
		require.Nil(t, err)

		proof3, err := cli.PlayTransactions(ctx, tree, treeKey, []*transactions.Transaction{txn3})
		require.Nil(t, err)
		assert.Equal(t, proof3.Tip, tree.Tip().Bytes())

	})

	t.Run("transactions played out of order succeed", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		cli, err := newClient(ctx, group, bootAddrs)
		require.Nil(t, err)

		treeKey, err := crypto.GenerateKey()
		require.Nil(t, err)
		nodeStore := nodestore.MustMemoryStore(ctx)

		testTree, err := consensus.NewSignedChainTree(ctx, treeKey.PublicKey, nodeStore)
		require.Nil(t, err)

		emptyTip := testTree.Tip()

		basisNodes0 := testhelpers.DagToByteNodes(t, testTree.ChainTree.Dag)

		blockWithHeaders0 := transactLocal(ctx, t, testTree, treeKey, 0, "down/in/the/tree", "atestvalue")
		tip0 := testTree.Tip()

		basisNodes1 := testhelpers.DagToByteNodes(t, testTree.ChainTree.Dag)

		blockWithHeaders1 := transactLocal(ctx, t, testTree, treeKey, 1, "other/thing", "sometestvalue")
		tip1 := testTree.Tip()

		respCh1, sub1 := transactRemote(ctx, t, cli, testTree.MustId(), blockWithHeaders1, tip1, basisNodes1, emptyTip)
		defer cli.UnsubscribeFromAbr(sub1)
		defer close(respCh1)

		respCh0, sub0 := transactRemote(ctx, t, cli, testTree.MustId(), blockWithHeaders0, tip0, basisNodes0, emptyTip)
		defer cli.UnsubscribeFromAbr(sub0)
		defer close(respCh0)

		resp0 := <-respCh0
		require.IsType(t, &gossip.Proof{}, resp0)

		resp1 := <-respCh1
		require.IsType(t, &gossip.Proof{}, resp1)

	})

	t.Run("invalid previous tip fails", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		clientA, err := newClient(ctx, group, bootAddrs)
		require.Nil(t, err)
		clientB, err := newClient(ctx, group, bootAddrs)
		require.Nil(t, err)

		treeKey, err := crypto.GenerateKey()
		require.Nil(t, err)
		nodeStoreA := nodestore.MustMemoryStore(ctx)
		nodeStoreB := nodestore.MustMemoryStore(ctx)

		testTreeA, err := consensus.NewSignedChainTree(ctx, treeKey.PublicKey, nodeStoreA)
		require.Nil(t, err)

		// establish different first valid transactions on 2 different local chaintrees
		transactLocal(ctx, t, testTreeA, treeKey, 0, "down/in/the/treeA", "atestvalue")
		basisNodesA1 := testhelpers.DagToByteNodes(t, testTreeA.ChainTree.Dag)

		testTreeB, err := consensus.NewSignedChainTree(ctx, treeKey.PublicKey, nodeStoreB)
		require.Nil(t, err)
		emptyTip := testTreeB.Tip()

		basisNodesB0 := testhelpers.DagToByteNodes(t, testTreeB.ChainTree.Dag)
		blockWithHeadersB0 := transactLocal(ctx, t, testTreeB, treeKey, 0, "down/in/the/treeB", "btestvalue")
		tipB0 := testTreeB.Tip()

		// run a second transaction on the first local chaintree
		blockWithHeadersA1 := transactLocal(ctx, t, testTreeA, treeKey, 1, "other/thing", "sometestvalue")
		tipA1 := testTreeA.Tip()

		/* Now send tx at height 1 from chaintree A followed by
		   tx at height 0 from chaintree B
		   tx at height 1 should be a byzantine transaction because its previous tip value
		   from chaintree A won't line up with tx at height 0 from chaintree B.
		   This can't be checked until after tx 0 is committed and this test is for
		   verifying that that happens and result is an invalid tx
		*/
		respCh1, sub1 := transactRemote(ctx, t, clientB, testTreeB.MustId(), blockWithHeadersA1, tipA1, basisNodesA1, emptyTip)
		defer clientB.UnsubscribeFromAbr(sub1)
		defer close(respCh1)

		time.Sleep(1 * time.Second)

		respCh0, sub0 := transactRemote(ctx, t, clientA, testTreeB.MustId(), blockWithHeadersB0, tipB0, basisNodesB0, emptyTip)
		defer clientA.UnsubscribeFromAbr(sub0)
		defer close(respCh0)

		resp0 := <-respCh0
		require.IsType(t, &gossip.Proof{}, resp0)

		// TODO: this is now a timeout error.
		// we can probably figure out a more elegant way to test this - like maybe sending in a successful 3rd transaction
		ticker := time.NewTimer(2 * time.Second)
		defer ticker.Stop()

		select {
		case <-respCh1:
			t.Fatalf("received a proof when we shouldn't have")
		case <-ticker.C:
			// yay a pass!
		}
	})

	t.Run("non-owner transactions fail", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		cli, err := newClient(ctx, group, bootAddrs)
		require.Nil(t, err)

		treeKey1, err := crypto.GenerateKey()
		require.Nil(t, err)
		nodeStore := nodestore.MustMemoryStore(ctx)
		chain, err := consensus.NewSignedChainTree(ctx, treeKey1.PublicKey, nodeStore)
		require.Nil(t, err)

		treeKey2, err := crypto.GenerateKey()
		require.Nil(t, err)

		// transaction with non-owner key should fail
		txn, err := chaintree.NewSetDataTransaction("down/in/the/thing", "sometestvalue")
		require.Nil(t, err)

		_, err = cli.PlayTransactions(ctx, chain, treeKey2, []*transactions.Transaction{txn})
		require.NotNil(t, err)
	})
}

func TestSubscriptions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ts := testnotarygroup.NewTestSet(t, groupMembers)
	group, nodes, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)
	require.Len(t, nodes, groupMembers)

	booter, err := p2p.NewHostFromOptions(ctx)
	require.Nil(t, err)

	bootAddrs := make([]string, len(booter.Addresses()))
	for i, addr := range booter.Addresses() {
		bootAddrs[i] = addr.String()
	}

	startNodes(t, ctx, nodes, bootAddrs)

	t.Run("subscribes to all rounds", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		cli, err := newClient(ctx, group, bootAddrs)
		require.Nil(t, err)

		treeKey, err := crypto.GenerateKey()
		require.Nil(t, err)
		nodeStore := nodestore.MustMemoryStore(ctx)

		testTree, err := consensus.NewSignedChainTree(ctx, treeKey.PublicKey, nodeStore)
		require.Nil(t, err)

		emptyTip := testTree.Tip()

		basisNodes0 := testhelpers.DagToByteNodes(t, testTree.ChainTree.Dag)
		blockWithHeaders0 := transactLocal(ctx, t, testTree, treeKey, 0, "down/in/the/tree", "atestvalue")
		tip0 := testTree.Tip()

		roundCh := make(chan *types.RoundConfirmationWrapper, 10)
		roundSubscription, err := cli.SubscribeToRounds(ctx, roundCh)
		require.Nil(t, err)
		defer cli.UnsubscribeFromRounds(roundSubscription)

		respCh0, sub0 := transactRemote(ctx, t, cli, testTree.MustId(), blockWithHeaders0, tip0, basisNodes0, emptyTip)
		defer cli.UnsubscribeFromAbr(sub0)
		defer close(respCh0)
		<-respCh0

		basisNodes1 := testhelpers.DagToByteNodes(t, testTree.ChainTree.Dag)
		blockWithHeaders1 := transactLocal(ctx, t, testTree, treeKey, 1, "other/thing", "sometestvalue")
		tip1 := testTree.Tip()

		respCh1, sub1 := transactRemote(ctx, t, cli, testTree.MustId(), blockWithHeaders1, tip1, basisNodes1, emptyTip)
		defer cli.UnsubscribeFromAbr(sub1)
		defer close(respCh1)
		<-respCh1

		require.Len(t, roundCh, 2)

		round1 := <-roundCh
		require.NotEqual(t, round1, cli.CurrentRound())
		round2 := <-roundCh
		require.Equal(t, round2, cli.CurrentRound())
	})

	t.Run("subscribes to dids", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		cli, err := newClient(ctx, group, bootAddrs)
		require.Nil(t, err)

		treeKey, err := crypto.GenerateKey()
		require.Nil(t, err)
		tree, err := consensus.NewSignedChainTree(ctx, treeKey.PublicKey, nodestore.MustMemoryStore(ctx))
		require.Nil(t, err)

		txn, err := chaintree.NewSetDataTransaction("down/in/the/thing", "sometestvalue")
		require.Nil(t, err)

		// first subscrie to the channel
		proofCh := make(chan *gossip.Proof, 10)
		cli.SubscribeToDid(ctx, tree.MustId(), proofCh)

		// then play a transaction
		playProof, err := cli.PlayTransactions(ctx, tree, treeKey, []*transactions.Transaction{txn})
		require.Nil(t, err)

		// assert that the subscription got an update
		subProof := <-proofCh
		require.Equal(t, playProof.AddBlockRequest.NewTip, subProof.AddBlockRequest.NewTip)

		// then play another transaction
		txn2, err := chaintree.NewSetDataTransaction("down/in/the/thing", "new value")
		require.Nil(t, err)

		playProof2, err := cli.PlayTransactions(ctx, tree, treeKey, []*transactions.Transaction{txn2})
		require.Nil(t, err)

		// and assert the subscription got that transaction
		subProof2 := <-proofCh
		require.Equal(t, playProof2.AddBlockRequest.NewTip, subProof2.AddBlockRequest.NewTip)

		// then make sure there aren't any updates left (since we haven't updated)
		require.Len(t, proofCh, 0)
	})
}

func TestGetLatest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ts := testnotarygroup.NewTestSet(t, groupMembers)
	group, nodes, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)
	require.Len(t, nodes, groupMembers)

	booter, err := p2p.NewHostFromOptions(ctx)
	require.Nil(t, err)

	bootAddrs := make([]string, len(booter.Addresses()))
	for i, addr := range booter.Addresses() {
		bootAddrs[i] = addr.String()
	}

	startNodes(t, ctx, nodes, bootAddrs)

	// This must remain the first Run in this test block
	t.Run("test known Err propogation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		cli1, err := newClient(ctx, group, bootAddrs)
		require.Nil(t, err)

		treeKey, err := crypto.GenerateKey()
		require.Nil(t, err)

		tree, err := cli1.GetLatest(ctx, consensus.EcdsaPubkeyToDid(treeKey.PublicKey))
		require.Nil(t, tree)
		require.Equal(t, err, ErrNoRound)

		startRounds(t, ctx, group, bootAddrs)

		tree, err = cli1.GetLatest(ctx, consensus.EcdsaPubkeyToDid(treeKey.PublicKey))
		require.Nil(t, tree)
		require.Equal(t, err, ErrTipNotFound)
	})

	t.Run("test basic setup", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// first play a transaction from one client to the notary group

		path := "down/in/the/thing"
		value := "sometestvalue"

		cli1, err := newClient(ctx, group, bootAddrs)
		require.Nil(t, err)

		treeKey, err := crypto.GenerateKey()
		require.Nil(t, err)
		tree, err := consensus.NewSignedChainTree(ctx, treeKey.PublicKey, nodestore.MustMemoryStore(ctx))
		require.Nil(t, err)

		txn, err := chaintree.NewSetDataTransaction(path, value)
		require.Nil(t, err)

		proof, err := cli1.PlayTransactions(ctx, tree, treeKey, []*transactions.Transaction{txn})
		require.Nil(t, err)
		assert.Equal(t, proof.Tip, tree.Tip().Bytes())

		// now create a new client (with a fresh store)

		cli2, err := newClient(ctx, group, bootAddrs)
		require.Nil(t, err)

		err = cli2.WaitForFirstRound(ctx, 2*time.Second)
		require.Nil(t, err)

		// get the tree using cli2 and make sure you can resolve the data
		cli2Tree, err := cli2.GetLatest(ctx, tree.MustId())
		require.Nil(t, err)

		require.Equal(t, proof.Tip, cli2Tree.Tip().Bytes())

		resp, remain, err := cli2Tree.ChainTree.Dag.Resolve(ctx, strings.Split("tree/data/"+path, "/"))
		t.Logf("remain: %v", remain)
		require.Nil(t, err)
		require.Equal(t, value, resp)
	})

	t.Run("test do a setData then another then resolve first", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		// first play a transaction from one client to the notary group

		cli1, err := newClient(ctx, group, bootAddrs)
		require.Nil(t, err)

		treeKey, err := crypto.GenerateKey()
		require.Nil(t, err)
		tree, err := consensus.NewSignedChainTree(ctx, treeKey.PublicKey, nodestore.MustMemoryStore(ctx))
		require.Nil(t, err)

		txn, err := chaintree.NewSetDataTransaction("first/path/is/deeper", true)
		require.Nil(t, err)

		proof, err := cli1.PlayTransactions(ctx, tree, treeKey, []*transactions.Transaction{txn})
		require.Nil(t, err)
		assert.Equal(t, proof.Tip, tree.Tip().Bytes())

		txn2, err := chaintree.NewSetDataTransaction("second/path", true)
		require.Nil(t, err)

		proof2, err := cli1.PlayTransactions(ctx, tree, treeKey, []*transactions.Transaction{txn2})
		require.Nil(t, err)
		assert.Equal(t, proof2.Tip, tree.Tip().Bytes())

		// now create a new client (with a fresh store)

		cli2, err := newClient(ctx, group, bootAddrs)
		require.Nil(t, err)

		err = cli2.WaitForFirstRound(ctx, 2*time.Second)
		require.Nil(t, err)

		// get the tree using cli2 and make sure you can resolve the data
		cli2Tree, err := cli2.GetLatest(ctx, tree.MustId())
		require.Nil(t, err)
		resp, remain, err := cli2Tree.ChainTree.Dag.Resolve(ctx, strings.Split("tree/data/first/path/is/deeper", "/"))
		t.Logf("remain: %v", remain)
		require.Nil(t, err)
		require.Equal(t, true, resp)
	})
}

func transactLocal(ctx context.Context, t testing.TB, tree *consensus.SignedChainTree, treeKey *ecdsa.PrivateKey, height uint64, path, value string) *chaintree.BlockWithHeaders {
	var pt *cid.Cid
	if !tree.IsGenesis() {
		tip := tree.Tip()
		pt = &tip
	}

	txn, err := chaintree.NewSetDataTransaction(path, value)
	require.Nil(t, err)
	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  pt,
			Height:       height,
			Transactions: []*transactions.Transaction{txn},
		},
	}

	blockWithHeaders, err := consensus.SignBlock(ctx, unsignedBlock, treeKey)
	require.Nil(t, err)

	_, err = tree.ChainTree.ProcessBlock(ctx, blockWithHeaders)
	require.Nil(t, err)

	return blockWithHeaders
}

func transactRemote(ctx context.Context, t testing.TB, client *Client, treeID string, blockWithHeaders *chaintree.BlockWithHeaders, newTip cid.Cid, stateNodes [][]byte, emptyTip cid.Cid) (chan *gossip.Proof, subscription) {
	sw := safewrap.SafeWrap{}

	var previousTipBytes []byte
	if blockWithHeaders.PreviousTip == nil {
		previousTipBytes = emptyTip.Bytes()
	} else {
		previousTipBytes = blockWithHeaders.PreviousTip.Bytes()
	}

	transMsg := &services.AddBlockRequest{
		PreviousTip: previousTipBytes,
		Height:      blockWithHeaders.Height,
		NewTip:      newTip.Bytes(),
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
		State:       stateNodes,
		ObjectId:    []byte(treeID),
	}

	t.Logf("sending remote transaction id: %s height: %d", base64.StdEncoding.EncodeToString(consensus.RequestID(transMsg)), transMsg.Height)

	resp := make(chan *gossip.Proof, 1)
	sub, err := client.SubscribeToAbr(ctx, transMsg, resp)
	require.Nil(t, err)

	err = client.SendWithoutWait(ctx, transMsg)
	require.Nil(t, err)

	return resp, sub
}

func TestClientGetTip(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ts := testnotarygroup.NewTestSet(t, groupMembers)
	group, nodes, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)
	require.Len(t, nodes, groupMembers)

	booter, err := p2p.NewHostFromOptions(ctx)
	require.Nil(t, err)

	bootAddrs := make([]string, len(booter.Addresses()))
	for i, addr := range booter.Addresses() {
		bootAddrs[i] = addr.String()
	}

	startNodes(t, ctx, nodes, bootAddrs)

	t.Run("test get existing tip", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		cli, err := newClient(ctx, group, bootAddrs)
		require.Nil(t, err)

		treeKey, err := crypto.GenerateKey()
		require.Nil(t, err)
		tree, err := consensus.NewSignedChainTree(ctx, treeKey.PublicKey, nodestore.MustMemoryStore(ctx))
		require.Nil(t, err)

		txn, err := chaintree.NewSetDataTransaction("down/in/the/thing", "sometestvalue")
		require.Nil(t, err)

		sendProof, err := cli.PlayTransactions(ctx, tree, treeKey, []*transactions.Transaction{txn})
		require.Nil(t, err)
		assert.Equal(t, sendProof.Tip, tree.Tip().Bytes())

		proof, err := cli.GetTip(ctx, tree.MustId())
		require.Nil(t, err)

		require.Equal(t, sendProof.Tip, proof.Tip)
	})

	t.Run("get non existant tip", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		cli, err := newClient(ctx, group, bootAddrs)
		require.Nil(t, err)
		treeKey, err := crypto.GenerateKey()
		require.Nil(t, err)
		tree, err := consensus.NewSignedChainTree(ctx, treeKey.PublicKey, nodestore.MustMemoryStore(ctx))
		require.Nil(t, err)

		txn, err := chaintree.NewSetDataTransaction("down/in/the/thing", "sometestvalue")
		require.Nil(t, err)

		sendProof, err := cli.PlayTransactions(ctx, tree, treeKey, []*transactions.Transaction{txn})
		require.Nil(t, err)
		assert.Equal(t, sendProof.Tip, tree.Tip().Bytes())

		_, err = cli.GetTip(ctx, "did:tupelo:doesnotexist")
		require.Equal(t, ErrTipNotFound, err)
	})
}

func TestGetTipAppropriatelyErrors(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ts := testnotarygroup.NewTestSet(t, groupMembers)
	group, nodes, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)
	require.Len(t, nodes, groupMembers)
	booter, err := p2p.NewHostFromOptions(ctx)
	require.Nil(t, err)

	bootAddrs := make([]string, len(booter.Addresses()))
	for i, addr := range booter.Addresses() {
		bootAddrs[i] = addr.String()
	}

	startNodes(t, ctx, nodes, bootAddrs)

	cli, err := newClient(ctx, group, bootAddrs)
	require.Nil(t, err)
	_, err = cli.GetTip(ctx, "did:doesntmatter")
	require.Equal(t, err, ErrNoRound)
}

func TestTokenTransactions(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ts := testnotarygroup.NewTestSet(t, groupMembers)
	group, nodes, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)
	require.Len(t, nodes, groupMembers)

	booter, err := p2p.NewHostFromOptions(ctx)
	require.Nil(t, err)

	bootAddrs := make([]string, len(booter.Addresses()))
	for i, addr := range booter.Addresses() {
		bootAddrs[i] = addr.String()
	}

	startNodes(t, ctx, nodes, bootAddrs)

	sendKey, err := crypto.GenerateKey()
	require.Nil(t, err)

	receiveKey, err := crypto.GenerateKey()
	require.Nil(t, err)

	t.Run("valid transaction", func(t *testing.T) {
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		sendCli, err := newClient(ctx, group, bootAddrs)
		require.Nil(t, err)

		sendTree, err := consensus.NewSignedChainTree(ctx, sendKey.PublicKey, nodestore.MustMemoryStore(ctx))
		require.Nil(t, err)

		receiveTree, err := consensus.NewSignedChainTree(ctx, receiveKey.PublicKey, nodestore.MustMemoryStore(ctx))
		require.Nil(t, err)

		tokenName := "test-token"
		tokenMax := uint64(50)
		mintAmount := uint64(25)
		sendTxId := "send-test-transaction"
		sendAmount := uint64(10)

		establishTxn, err := chaintree.NewEstablishTokenTransaction(tokenName, tokenMax)
		require.Nil(t, err)

		mintTxn, err := chaintree.NewMintTokenTransaction(tokenName, mintAmount)
		require.Nil(t, err)

		sendTxn, err := chaintree.NewSendTokenTransaction(sendTxId, tokenName, sendAmount, receiveTree.MustId())
		require.Nil(t, err)

		senderTxns := []*transactions.Transaction{establishTxn, mintTxn, sendTxn}

		sendProof, err := sendCli.PlayTransactions(ctx, sendTree, sendKey, senderTxns)
		require.Nil(t, err)
		assert.Equal(t, sendProof.Tip, sendTree.Tip().Bytes())

		fullTokenName := &consensus.TokenName{ChainTreeDID: sendTree.MustId(), LocalName: tokenName}
		tokenPayload, err := consensus.TokenPayloadForTransaction(sendTree.ChainTree, fullTokenName, sendTxId, sendProof)
		require.Nil(t, err)
		assert.Equal(t, tokenPayload.Tip, sendTree.Tip().String())

		receiveCli, err := newClient(ctx, group, bootAddrs)
		require.Nil(t, err)

		tipCid, err := cid.Decode(tokenPayload.Tip)
		require.Nil(t, err)

		receiveTxn, err := chaintree.NewReceiveTokenTransaction(sendTxId, tipCid.Bytes(), tokenPayload.Proof, tokenPayload.Leaves)
		require.Nil(t, err)

		receiverTxn := []*transactions.Transaction{receiveTxn}

		_, err = receiveCli.PlayTransactions(ctx, receiveTree, receiveKey, receiverTxn)
		require.Nil(t, err)
	})
}

func TestChaintreeOwnsOtherChaintree(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ts := testnotarygroup.NewTestSet(t, groupMembers)
	group, nodes, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)
	require.Len(t, nodes, groupMembers)

	booter, err := p2p.NewHostFromOptions(ctx)
	require.Nil(t, err)

	bootAddrs := make([]string, len(booter.Addresses()))
	for i, addr := range booter.Addresses() {
		bootAddrs[i] = addr.String()
	}

	startNodes(t, ctx, nodes, bootAddrs)

	ownerKey, err := crypto.GenerateKey()
	require.Nil(t, err)

	ownedKey, err := crypto.GenerateKey()
	require.Nil(t, err)

	ownerTree, err := consensus.NewSignedChainTree(ctx, ownerKey.PublicKey, nodestore.MustMemoryStore(ctx))
	require.Nil(t, err)

	tupelo, err := newClient(ctx, group, bootAddrs)
	require.Nil(t, err)

	// run a setData on ownerTree so it's in the HAMT
	setDataTxn, err := chaintree.NewSetDataTransaction("down/in/the/thing", "hi")
	require.Nil(t, err)
	_, err = tupelo.PlayTransactions(ctx, ownerTree, ownerKey, []*transactions.Transaction{setDataTxn})
	require.Nil(t, err)

	ownedTree, err := consensus.NewSignedChainTree(ctx, ownedKey.PublicKey, nodestore.MustMemoryStore(ctx))
	require.Nil(t, err)

	ownerTxn, err := chaintree.NewSetOwnershipTransaction([]string{ownerTree.MustId()})
	require.Nil(t, err)

	ownProof, err := tupelo.PlayTransactions(ctx, ownedTree, ownedKey, []*transactions.Transaction{ownerTxn})
	require.Nil(t, err)
	assert.Equal(t, ownProof.Tip, ownedTree.Tip().Bytes())

	// ownedKey can no longer modify
	_, err = tupelo.PlayTransactions(ctx, ownedTree, ownedKey, []*transactions.Transaction{setDataTxn})
	require.NotNil(t, err)

	// but ownerKey can
	setDataProof, err := tupelo.PlayTransactions(ctx, ownedTree, ownerKey, []*transactions.Transaction{setDataTxn})
	require.Nil(t, err)
	assert.Equal(t, setDataProof.Tip, ownedTree.Tip().Bytes())

	// TODO: Change ownership of ownerTree and verify that new owner can change ownedTree and old owner cannot
}
