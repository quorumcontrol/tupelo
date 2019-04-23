// +build integration

package gossip3

import (
	"reflect"
	"context"
	"crypto/ecdsa"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-cid"
	libp2plogging "github.com/ipfs/go-log"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo-go-sdk/client"
	"github.com/quorumcontrol/tupelo-go-sdk/consensus"
	extmsgs "github.com/quorumcontrol/tupelo-go-sdk/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
	"github.com/quorumcontrol/tupelo/gossip3/actors"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testRootPath    = "./.tmp"
	testCommitPath  = testRootPath + "/teststore/commit"
	testCurrentPath = testRootPath + "/teststore/current"
)

func dagToByteNodes(t *testing.T, dagTree *dag.Dag) [][]byte {
	cborNodes, err := dagTree.Nodes()
	require.Nil(t, err)
	nodes := make([][]byte, len(cborNodes))
	for i, node := range cborNodes {
		nodes[i] = node.RawData()
	}
	return nodes
}

func newValidTransaction(t *testing.T) extmsgs.Transaction {
	sw := safewrap.SafeWrap{}
	treeKey, err := crypto.GenerateKey()
	require.Nil(t, err)

	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: nil,
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]string{
						"path":  "down/in/the/thing",
						"value": "hi",
					},
				},
			},
		},
	}

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	emptyTree := consensus.NewEmptyTree(treeDID, nodeStore)
	emptyTip := emptyTree.Tip
	testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)
	require.Nil(t, err)

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	require.Nil(t, err)

	_, err = testTree.ProcessBlock(blockWithHeaders)
	require.Nil(t, err)
	nodes := dagToByteNodes(t, emptyTree)
	return extmsgs.Transaction{
		State:       nodes,
		PreviousTip: emptyTip.Bytes(),
		NewTip:      testTree.Dag.Tip.Bytes(),
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
		ObjectID:    []byte(treeDID),
	}
}

func newSystemWithRemotes(ctx context.Context, bootstrap p2p.Node, indexOfLocal int, testSet *testnotarygroup.TestSet) (*types.Signer, *types.NotaryGroup, error) {
	ng := types.NewNotaryGroup("test notary")

	localSigner := types.NewLocalSigner(testSet.PubKeys[indexOfLocal].ToEcdsaPub(), testSet.SignKeys[indexOfLocal])
	commitPath := testCommitPath + "/" + localSigner.ID
	currentPath := testCurrentPath + "/" + localSigner.ID
	if err := os.MkdirAll(commitPath, 0755); err != nil {
		return nil, nil, err
	}

	if err := os.MkdirAll(currentPath, 0755); err != nil {
		return nil, nil, err
	}

	currentStore, err := storage.NewBadgerStorage(currentPath)
	if err != nil {
		return nil, nil, fmt.Errorf("error badgering: %v", err)
	}

	bootAddrs := testnotarygroup.BootstrapAddresses(bootstrap)

	node, err := p2p.NewLibP2PHost(ctx, testSet.EcdsaKeys[indexOfLocal], 0)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating p2p node")
	}
	if _, err = node.Bootstrap(bootAddrs); err != nil {
		return nil, nil, err
	}
	remote.NewRouter(node)

	syncer, err := actor.SpawnNamed(actors.NewTupeloNodeProps(&actors.TupeloConfig{
		Self:              localSigner,
		NotaryGroup:       ng,
		CurrentStateStore: currentStore,
		PubSubSystem:      remote.NewNetworkPubSub(node),
	}), "tupelo-"+localSigner.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("error spawning: %v", err)
	}
	localSigner.Actor = syncer
	go func() {
		<-ctx.Done()
		actor.EmptyRootContext.Stop(syncer)
	}()
	ng.AddSigner(localSigner)

	for i, verKey := range testSet.VerKeys {
		if i != indexOfLocal {
			// this is a remote signer
			signer := types.NewRemoteSigner(testSet.PubKeys[i].ToEcdsaPub(), verKey)
			signer.Actor = actor.NewPID(signer.ActorAddress(localSigner.DstKey), "tupelo-"+signer.ID)
			ng.AddSigner(signer)
		}
	}
	return localSigner, ng, nil
}

func TestLibP2PSigning(t *testing.T) {
	paths := []string{
		testCommitPath,
		testCurrentPath,
	}
	for _, path := range paths {
		err := os.MkdirAll(path, 0755)
		require.Nil(t, err)
	}
	defer os.RemoveAll(testRootPath)

	remote.Start()
	defer remote.Stop()

	numMembers := 20
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		middleware.Log.Infow("---- tests over ----")
		cancel()
	}()
	ts := testnotarygroup.NewTestSet(t, numMembers)

	bootstrap := testnotarygroup.NewBootstrapHost(ctx, t)
	bootAddrs := testnotarygroup.BootstrapAddresses(bootstrap)

	localSyncers := make([]*actor.PID, numMembers)
	systems := make([]*types.NotaryGroup, numMembers)
	for i := 0; i < numMembers; i++ {
		local, ng, err := newSystemWithRemotes(ctx, bootstrap, i, ts)
		require.Nil(t, err)
		systems[i] = ng
		localSyncers[i] = local.Actor
		signers := ng.AllSigners()
		require.Len(t, signers, numMembers)
	}

	err := libp2plogging.SetLogLevel("swarm2", "ERROR")
	require.Nil(t, err)
	time.Sleep(100 * time.Millisecond) // give time for bootstrap

	clientKey, err := crypto.GenerateKey()
	require.Nil(t, err)
	clientHost, err := p2p.NewLibP2PHost(ctx, clientKey, 0)
	require.Nil(t, err)
	_, err = clientHost.Bootstrap(bootAddrs)
	require.Nil(t, err)
	err = clientHost.WaitForBootstrap(2, 1*time.Second)
	require.Nil(t, err)

	pubSub := remote.NewNetworkPubSub(clientHost)

	remote.NewRouter(clientHost)

	wg := sync.WaitGroup{}
	wg.Add(numMembers)

	for i := 0; i < 100; i++ {
		trans := newValidTransaction(t)
		cli := client.New(systems[0], string(trans.ObjectID), pubSub)
		err := cli.SendTransaction(&trans)
		require.Nil(t, err)
	}

	trans := newValidTransaction(t)

	cli := client.New(systems[0], string(trans.ObjectID), pubSub)
	cli.Listen()
	defer cli.Stop()

	fut := cli.Subscribe(&trans, 90*time.Second)

	err = cli.SendTransaction(&trans)
	require.Nil(t, err)

	resp, err := fut.Result()
	require.Nil(t, err)
	require.NotNil(t, resp)
	require.IsType(t, &extmsgs.CurrentState{}, resp)
	sigResp := resp.(*extmsgs.CurrentState)
	assert.Equal(t, sigResp.Signature.NewTip, trans.NewTip)
}

func sendTransaction(cli *client.Client, group *types.NotaryGroup,
	treeKey *ecdsa.PrivateKey, treeDID string, tree *chaintree.ChainTree, emptyTip cid.Cid,
	height uint64) error {
	trans, err := newValidSuccessiveTransaction(treeKey, treeDID, tree, emptyTip, height)
	if err != nil {
		return err
	}
	newTip, err := cid.Cast(trans.NewTip)
	if err != nil {
		return err
	}
	prevTip, err := cid.Cast(trans.PreviousTip)
	if err != nil {
		return err
	}

	fut := cli.Subscribe(trans, 90*time.Second)

	middleware.Log.Debugw("sending transaction for confirmation by signers", "height", height,
		"tip", newTip, "prevTip", prevTip)
	if err = cli.SendTransaction(trans); err != nil {
		return err
	}

	start := time.Now()
	middleware.Log.Debugw("waiting for response on transaction subscription")
	resp, err := fut.Result()
	if err != nil {
		return fmt.Errorf("received error from transaction subscription: %s", err)
	}
	if resp == nil {
		return fmt.Errorf("received nil response from transaction subscription")
	}
	curState, ok := resp.(*extmsgs.CurrentState)
	if !ok {
		err, ok := resp.(*extmsgs.Error)
		if ok {
			return fmt.Errorf("got an error from transaction subscription: %v", err)
		}
		return fmt.Errorf(
			"got something else than CurrentState message from transaction subscription: %s",
		 	reflect.TypeOf(resp))
	}

	if !reflect.DeepEqual(curState.Signature.NewTip, trans.NewTip) {
		return fmt.Errorf("current state tip doesn't correspond to the transaction we sent")
	}

	stop := time.Now()
	middleware.Log.Infow("received confirmation of transaction from signer subscription\n",
		"timeTaken", stop.Sub(start).Seconds())

	return nil
}

func newValidSuccessiveTransaction(treeKey *ecdsa.PrivateKey, treeDID string,
	tree *chaintree.ChainTree, emptyTip cid.Cid, height uint64) (*extmsgs.Transaction, error) {
	sw := safewrap.SafeWrap{}

	value := time.Now().Format(time.RFC3339)
	var prevTipP *cid.Cid
	var prevTipB []byte
	if tree.Dag.Tip != emptyTip {
		prevTipP = &tree.Dag.Tip
		prevTipB = tree.Dag.Tip.Bytes()
	} else {
		prevTipB = emptyTip.Bytes()
	}
	middleware.Log.Debugw("creating new transaction", "path", "testTime", "value",
		value, "treeDID", treeDID)

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			Height:      height,
			PreviousTip: prevTipP,
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]string{
						"path":  "testTime",
						"value": value,
					},
				},
			},
		},
	}

	middleware.Log.Debugw("signing block")
	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	if err != nil {
		return nil, err
	}

	middleware.Log.Debugw("telling tree to process block")
	valid, err := tree.ProcessBlock(blockWithHeaders)
	if err != nil {
		return nil, err
	}
	if !valid {
		return nil, fmt.Errorf("invalid result from block processing")
	}

	cborNodes, err := tree.Dag.Nodes()
	if err != nil {
		return nil, err
	}
	nodes := make([][]byte, len(cborNodes))
	for i, node := range cborNodes {
		nodes[i] = node.RawData()
	}

	return &extmsgs.Transaction{
		Height:      height,
		State:       nodes,
		PreviousTip: prevTipB,
		NewTip:      tree.Dag.Tip.Bytes(),
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
		ObjectID:    []byte(treeDID),
	}, nil
}

func setUpSystem(t *testing.T) (*remote.NetworkPubSub, *types.NotaryGroup, func(), error)  {
	cleanupFuncs := []func(){}
	cleanUp := func() {
		middleware.Log.Infow("---- tests over ----")
		os.RemoveAll(testRootPath)

		for _, f := range cleanupFuncs {
			f()
		}
	}
	
	if err := os.MkdirAll(testCurrentPath, 0755); err != nil {
		return nil, nil, cleanUp, err
	}

	remote.Start()
	cleanupFuncs = append(cleanupFuncs, remote.Stop)

	numMembers := 3
	ctx, cancel := context.WithCancel(context.Background())
	cleanupFuncs = append(cleanupFuncs, cancel)
	ts := testnotarygroup.NewTestSet(t, numMembers)

	bootstrap := testnotarygroup.NewBootstrapHost(ctx, t)
	bootAddrs := testnotarygroup.BootstrapAddresses(bootstrap)

	localSyncers := make([]*actor.PID, numMembers)
	systems := make([]*types.NotaryGroup, numMembers)
	for i := 0; i < numMembers; i++ {
		local, ng, err := newSystemWithRemotes(ctx, bootstrap, i, ts)
		if err != nil {
			return nil, nil, cleanUp, err
		}
		systems[i] = ng
		localSyncers[i] = local.Actor
		signers := ng.AllSigners()
		if len(signers) != numMembers {
			panic("number of signers != numMembers")
		}
	}

	clientKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, nil, cleanUp, err
	}
	clientHost, err := p2p.NewLibP2PHost(ctx, clientKey, 0)
	if err != nil {
		return nil, nil, cleanUp, err
	}
	_, err = clientHost.Bootstrap(bootAddrs)
	if err != nil {
		return nil, nil, cleanUp, err
	}
	err = clientHost.WaitForBootstrap(2, 1*time.Second)
	if err != nil {
		return nil, nil, cleanUp, err
	}

	remote.NewRouter(clientHost)
	pubSub := remote.NewNetworkPubSub(clientHost)

	return pubSub, systems[0], cleanUp, nil
}

// Test successive transactions on a single chaintree.
func TestSuccessiveTransactionsSingleTree(t *testing.T) {
	pubSub, group, cleanUp, err := setUpSystem(t)
	defer cleanUp()
	require.Nil(t, err)

	treeKey, err := crypto.GenerateKey()
	require.Nil(t, err)
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())
	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	emptyTree := consensus.NewEmptyTree(treeDID, nodeStore)
	emptyTip := emptyTree.Tip
	testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)
	require.Nil(t, err)

	cli := client.New(group, treeDID, pubSub)
	cli.Listen()
	defer cli.Stop()

	for h := uint64(0); h < 30; h++ {
		err = sendTransaction(cli, group, treeKey, treeDID, testTree, emptyTip, h)
		require.Nil(t, err)

		// TODO: currently if any syncer gets a transaction it perceives as "invalid" it will send
		// it back an error even if the system is doing ok. Once the errors are limited to the
		// rewards committee we can (hopefully) remove this sleep.
		middleware.Log.Infow("sleeping before sending next transaction")
		time.Sleep(10 * time.Millisecond)
	}
}
