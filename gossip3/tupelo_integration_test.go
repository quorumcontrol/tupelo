// +build integration

package gossip3

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	libp2plogging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	"github.com/quorumcontrol/tupelo/gossip3/actors"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/remote"
	"github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSystemWithRemotes(ctx context.Context, indexOfLocal int, testSet *testnotarygroup.TestSet) (*types.Signer, *types.NotaryGroup, error) {
	ng := types.NewNotaryGroup()

	localSigner := types.NewLocalSigner(testSet.PubKeys[indexOfLocal].ToEcdsaPub(), testSet.SignKeys[indexOfLocal])
	syncer, err := actor.SpawnNamed(actors.NewTupeloNodeProps(localSigner, ng), "tupelo-"+localSigner.ID)
	if err != nil {
		return nil, nil, fmt.Errorf("error spawning: %v", err)
	}
	localSigner.Actor = syncer
	go func() {
		<-ctx.Done()
		syncer.Poison()
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

func createHostsAndBridges(ctx context.Context, t *testing.T, testSet *testnotarygroup.TestSet) {
	bootstrap := testnotarygroup.NewBootstrapHost(ctx, t)
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
		for j, insideKey := range testSet.PubKeys {
			if i != j {
				remote.RegisterBridge(node.Identity(), insideKey.ToEcdsaPub())
			}
		}
	}
}

func TestLibP2PSigning(t *testing.T) {
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
	createHostsAndBridges(ctx, t, ts)
	libp2plogging.SetLogLevel("swarm2", "ERROR")
	time.Sleep(100 * time.Millisecond) // give time for bootstrap

	wg := sync.WaitGroup{}
	wg.Add(numMembers)

	trans := newValidTransaction(t)
	bits, err := trans.MarshalMsg(nil)
	require.Nil(t, err)
	id := crypto.Keccak256(bits)

	key := id
	middleware.Log.Infow("tests", "key", key)
	value := bits

	for i := 0; i < 50; i++ {
		trans := newValidTransaction(t)
		bits, err := trans.MarshalMsg(nil)
		require.Nil(t, err)
		key := crypto.Keccak256(bits)
		localSyncers[rand.Intn(len(localSyncers))].Tell(&messages.Store{
			Key:   key,
			Value: bits,
		})
		if err != nil {
			t.Fatalf("error sending transaction: %v", err)
		}
	}

	for _, s := range localSyncers {
		s.Tell(&messages.StartGossip{})
	}
	time.Sleep(100 * time.Millisecond) // give time for warmup

	start := time.Now()

	localSyncers[0].Tell(&messages.Store{
		Key:   key,
		Value: value,
	})

	var stop time.Time
	for {
		if (time.Now().Sub(start)) > (60 * time.Second) {
			t.Fatal("timed out looking for done function")
			break
		}
		val, err := localSyncers[0].RequestFuture(&messages.GetTip{ObjectID: trans.ObjectID}, 5*time.Second).Result()
		require.Nil(t, err)

		if len(val.([]byte)) > 0 {
			var currState messages.CurrentState
			_, err := currState.UnmarshalMsg(val.([]byte))
			require.Nil(t, err)
			if bytes.Equal(currState.Signature.NewTip, trans.NewTip) {
				stop = time.Now()
				break
			}
		}

		time.Sleep(200 * time.Millisecond)
	}

	t.Logf("Confirmation took %f seconds\n", stop.Sub(start).Seconds())
	assert.True(t, stop.Sub(start) < 60*time.Second)
}
