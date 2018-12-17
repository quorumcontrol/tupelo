// +build integration

package gossip3

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo/gossip3/actors"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/remote"
	"github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
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

	numMembers := 5
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

		// for y := 0; y < numMembers; i++ {
		// 	targets, err := ng.RewardsCommittee([]byte("lkasdjflkasdjfasdjfksldakfjaskdfj"), signers[y])
		// 	require.Nil(t, err)
		// 	require.Len(t, targets, 3)
		// }

	}
	createHostsAndBridges(ctx, t, ts)

	wg := sync.WaitGroup{}
	wg.Add(numMembers)

	trans := newValidTransaction(t)
	bits, err := trans.MarshalMsg(nil)
	require.Nil(t, err)
	id := crypto.Keccak256(bits)

	key := id
	middleware.Log.Infow("tests", "key", key)
	value := bits

	localSyncers[0].Tell(&messages.Store{
		Key:   key,
		Value: value,
	})

	for _, s := range localSyncers {
		s.Tell(&messages.StartGossip{})
		b := s
		go func(syncer *actor.PID) {
			val, err := syncer.RequestFuture(&messages.GetTip{ObjectID: trans.ObjectID}, 1*time.Second).Result()
			require.Nil(t, err)

			timer := time.AfterFunc(5*time.Second, func() {
				t.Logf("TIMEOUT %s", syncer.GetId())
				wg.Done()
				t.Fatalf("timeout waiting for key to appear")
			})
			var isDone bool
			for len(val.([]byte)) == 0 || !isDone {
				if len(val.([]byte)) > 0 {
					var currState messages.CurrentState
					_, err := currState.UnmarshalMsg(val.([]byte))
					require.Nil(t, err)
					if bytes.Equal(currState.Tip, trans.NewTip) {
						isDone = true
						continue
					}
				}

				val, err = syncer.RequestFuture(&messages.GetTip{ObjectID: trans.ObjectID}, 1*time.Second).Result()
				require.Nil(t, err)
				time.Sleep(200 * time.Millisecond)
			}
			timer.Stop()
			wg.Done()
		}(b)
	}
	wg.Wait()
}
