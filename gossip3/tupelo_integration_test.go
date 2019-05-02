// +build integration

package gossip3

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
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
		syncer.Stop()
	}()
	ng.AddSigner(localSigner)

	for i, verKey := range testSet.VerKeys {
		var signer *types.Signer
		if i != indexOfLocal {
			// this is a remote signer
			signer = types.NewRemoteSigner(testSet.PubKeys[i].ToEcdsaPub(), verKey)
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
