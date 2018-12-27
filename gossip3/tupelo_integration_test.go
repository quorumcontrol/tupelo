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
	libp2plogging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/gossip3/actors"
	"github.com/quorumcontrol/tupelo/gossip3/client"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/remote"
	"github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	testRootPath    = "./.tmp"
	testCommitPath  = testRootPath + "/teststore/commit"
	testCurrentPath = testRootPath + "/teststore/current"
)

func newSystemWithRemotes(ctx context.Context, indexOfLocal int, testSet *testnotarygroup.TestSet) (*types.Signer, *types.NotaryGroup, error) {
	ng := types.NewNotaryGroup("test notary")

	localSigner := types.NewLocalSigner(testSet.PubKeys[indexOfLocal].ToEcdsaPub(), testSet.SignKeys[indexOfLocal])
	commitPath := testCommitPath + "/" + localSigner.ID
	currentPath := testCurrentPath + "/" + localSigner.ID
	os.MkdirAll(commitPath, 0755)
	os.MkdirAll(currentPath, 0755)

	commitStore, err := storage.NewBadgerStorage(commitPath)
	if err != nil {
		return nil, nil, fmt.Errorf("error badgering: %v", err)
	}
	currenStore, err := storage.NewBadgerStorage(currentPath)
	if err != nil {
		return nil, nil, fmt.Errorf("error badgering: %v", err)
	}

	syncer, err := actor.SpawnNamed(actors.NewTupeloNodeProps(&actors.TupeloConfig{
		Self:              localSigner,
		NotaryGroup:       ng,
		CommitStore:       commitStore,
		CurrentStateStore: currenStore,
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
			signer.Actor = actor.NewPID(signer.ActorAddress(localSigner), "tupelo-"+signer.ID)
			ng.AddSigner(signer)
		}
	}
	return localSigner, ng, nil
}

func createHostsAndBridges(ctx context.Context, t *testing.T, bootstrap p2p.Node, testSet *testnotarygroup.TestSet) {
	bootAddrs := testnotarygroup.BootstrapAddresses(bootstrap)

	nodes := make([]p2p.Node, len(testSet.EcdsaKeys), len(testSet.EcdsaKeys))
	for i, key := range testSet.EcdsaKeys {
		node, err := p2p.NewLibP2PHost(ctx, key, 0)
		if err != nil {
			t.Fatalf("error creating libp2p host: %v", err)
		}
		node.Bootstrap(bootAddrs)
		nodes[i] = node
		remote.NewRouter(node)
	}
	for _, node := range nodes {
		node.WaitForBootstrap(1 * time.Second)
	}
}

func TestLibP2PSigning(t *testing.T) {
	paths := []string{
		testCommitPath,
		testCurrentPath,
	}
	for _, path := range paths {
		os.MkdirAll(path, 0755)
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

	localSyncers := make([]*actor.PID, numMembers, numMembers)
	systems := make([]*types.NotaryGroup, numMembers, numMembers)
	for i := 0; i < numMembers; i++ {
		local, ng, err := newSystemWithRemotes(ctx, i, ts)
		require.Nil(t, err)
		systems[i] = ng
		localSyncers[i] = local.Actor
		signers := ng.AllSigners()
		require.Len(t, signers, numMembers)
	}
	bootstrap := testnotarygroup.NewBootstrapHost(ctx, t)
	bootAddrs := testnotarygroup.BootstrapAddresses(bootstrap)

	createHostsAndBridges(ctx, t, bootstrap, ts)
	libp2plogging.SetLogLevel("swarm2", "ERROR")
	time.Sleep(100 * time.Millisecond) // give time for bootstrap

	clientKey, err := crypto.GenerateKey()
	require.Nil(t, err)
	clientHost, err := p2p.NewLibP2PHost(ctx, clientKey, 0)
	require.Nil(t, err)
	clientHost.Bootstrap(bootAddrs)
	clientHost.WaitForBootstrap(1 * time.Second)

	remote.NewRouter(clientHost)
	client := client.New(systems[0])

	wg := sync.WaitGroup{}
	wg.Add(numMembers)

	for i := 0; i < 100; i++ {
		trans := newValidTransaction(t)
		err := client.SendTransaction(systems[0].GetRandomSigner(), &trans)
		require.Nil(t, err)
	}

	trans := newValidTransaction(t)

	for _, s := range localSyncers {
		s.Tell(&messages.StartGossip{})
	}
	time.Sleep(200 * time.Millisecond) // give time for warmup

	ch, err := client.Subscribe(systems[0].AllSigners()[0], string(trans.ObjectID), 60*time.Second)
	require.Nil(t, err)

	client.SendTransaction(systems[0].GetRandomSigner(), &trans)
	start := time.Now()

	resp := <-ch
	require.NotNil(t, resp)
	stop := time.Now()
	assert.Equal(t, resp.Signature.NewTip, trans.NewTip)

	t.Logf("Confirmation took %f seconds\n", stop.Sub(start).Seconds())
	assert.True(t, stop.Sub(start) < 60*time.Second)
}
