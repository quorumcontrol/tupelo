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
	"github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/require"
)

func newTupeloSystem(ctx context.Context, testSet *testnotarygroup.TestSet) (*types.NotaryGroup, error) {
	ng := types.NewNotaryGroup()
	for i, signKey := range testSet.SignKeys {
		sk := signKey
		signer := types.NewLocalSigner(testSet.PubKeys[i].ToEcdsaPub(), sk)
		syncer, err := actor.SpawnNamed(actors.NewTupeloNodeProps(signer, ng), "tupelo-"+signer.ID)
		if err != nil {
			return nil, fmt.Errorf("error spawning: %v", err)
		}
		signer.Actor = syncer
		go func() {
			<-ctx.Done()
			syncer.Poison()
		}()
		ng.AddSigner(signer)
	}
	return ng, nil
}

func TestTupeloMemStorage(t *testing.T) {
	numMembers := 3
	ts := testnotarygroup.NewTestSet(t, numMembers)

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		middleware.Log.Infow("---- tests over ----")
		cancel()
	}()
	system, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)

	require.Len(t, system.Signers, numMembers)
	syncer := system.AllSigners()[0].Actor

	trans := newValidTransaction(t)
	bits, err := trans.MarshalMsg(nil)
	require.Nil(t, err)
	id := crypto.Keccak256(bits)

	key := id
	value := bits

	syncer.Tell(&messages.Store{
		Key:   key,
		Value: value,
	})

	time.Sleep(10 * time.Millisecond)

	val, err := syncer.RequestFuture(&messages.Get{Key: key}, 1*time.Second).Result()
	require.Nil(t, err)
	require.Equal(t, value, val)
}

func TestCommits(t *testing.T) {
	numMembers := 200
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		middleware.Log.Infow("---- tests over ----")
		cancel()
	}()
	ts := testnotarygroup.NewTestSet(t, numMembers)

	system, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)

	syncers := system.AllSigners()
	require.Len(t, system.Signers, numMembers)

	wg := sync.WaitGroup{}
	wg.Add(numMembers)

	trans := newValidTransaction(t)
	bits, err := trans.MarshalMsg(nil)
	require.Nil(t, err)
	id := crypto.Keccak256(bits)

	key := id
	middleware.Log.Infow("tests", "key", key)
	value := bits

	syncers[0].Actor.Tell(&messages.Store{
		Key:   key,
		Value: value,
	})
	syncers[1].Actor.Tell(&messages.Store{
		Key:   key,
		Value: value,
	})
	syncers[2].Actor.Tell(&messages.Store{
		Key:   key,
		Value: value,
	})

	for _, s := range syncers {
		s.Actor.Tell(&messages.StartGossip{})
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
		}(b.Actor)
	}
	wg.Wait()
}
