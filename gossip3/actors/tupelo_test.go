package actors

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/eventstream"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTupeloSystem(ctx context.Context, testSet *testnotarygroup.TestSet) (*types.NotaryGroup, error) {
	ng := types.NewNotaryGroup()
	for i, signKey := range testSet.SignKeys {
		sk := signKey
		signer := types.NewLocalSigner(testSet.PubKeys[i].ToEcdsaPub(), sk)
		syncer, err := actor.SpawnNamed(NewTupeloNodeProps(&TupeloConfig{
			Self:              signer,
			NotaryGroup:       ng,
			CommitStore:       storage.NewMemStorage(),
			CurrentStateStore: storage.NewMemStorage(),
		}), "tupelo-"+signer.ID)
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
func TestCommits(t *testing.T) {
	numMembers := 20
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
	t.Logf("syncer 0 id: %s", syncers[0].ID)

	for i := 0; i < 100; i++ {
		trans := newValidTransaction(t)
		bits, err := trans.MarshalMsg(nil)
		require.Nil(t, err)
		key := crypto.Keccak256(bits)
		syncers[rand.Intn(len(syncers))].Actor.Tell(&messages.Store{
			Key:   key,
			Value: bits,
		})
		syncers[rand.Intn(len(syncers))].Actor.Tell(&messages.Store{
			Key:   key,
			Value: bits,
		})
		syncers[rand.Intn(len(syncers))].Actor.Tell(&messages.Store{
			Key:   key,
			Value: bits,
		})
		if err != nil {
			t.Fatalf("error sending transaction: %v", err)
		}
	}

	for _, s := range syncers {
		s.Actor.Tell(&messages.StartGossip{})
	}

	t.Run("removes bad transactions", func(t *testing.T) {
		trans := newValidTransaction(t)
		bits, err := trans.MarshalMsg(nil)
		require.Nil(t, err)
		bits = append([]byte{byte(1)}, bits...) // append a bad byte
		id := crypto.Keccak256(bits)
		syncers[0].Actor.Tell(&messages.Store{
			Key:   id,
			Value: bits,
		})
		ret, err := syncers[0].Actor.RequestFuture(&messages.Get{Key: id}, 5*time.Second).Result()
		require.Nil(t, err)
		assert.Equal(t, ret, bits)

		// wait for it to get removed in the sync
		time.Sleep(100 * time.Millisecond)

		ret, err = syncers[0].Actor.RequestFuture(&messages.Get{Key: id}, 5*time.Second).Result()
		require.Nil(t, err)
		assert.Empty(t, ret)
	})

	t.Run("commits a good transaction", func(t *testing.T) {
		trans := newValidTransaction(t)
		bits, err := trans.MarshalMsg(nil)
		require.Nil(t, err)
		key := crypto.Keccak256(bits)

		fut := actor.NewFuture(10 * time.Second)
		sub := eventstream.Subscribe(func(evt interface{}) {
			fut.PID().Tell(evt)
		})
		defer eventstream.Unsubscribe(sub)

		sub.WithPredicate(func(evt interface{}) bool {
			switch msg := evt.(type) {
			case *messages.CurrentStateWrapper:
				if bytes.Equal(msg.CurrentState.Signature.TransactionID, key) {
					return true
				}
			}
			return false
		})

		syncers[rand.Intn(len(syncers))].Actor.Tell(&messages.Store{
			Key:   key,
			Value: bits,
		})

		_, err = fut.Result()
		require.Nil(t, err)
	})

	t.Run("reaches another node", func(t *testing.T) {
		trans := newValidTransaction(t)
		bits, err := trans.MarshalMsg(nil)
		require.Nil(t, err)
		key := crypto.Keccak256(bits)

		syncers[0].Actor.Tell(&messages.Store{
			Key:   key,
			Value: bits,
		})

		start := time.Now()
		var stop time.Time
		for {
			if time.Now().Sub(start) > 15*time.Second {
				t.Fatalf("timeout looking for done")
				break
			}
			val, err := syncers[0].Actor.RequestFuture(&messages.GetTip{ObjectID: trans.ObjectID}, 1*time.Second).Result()
			require.Nil(t, err)
			if len(val.([]byte)) > 0 {
				stop = time.Now()
				break
			}
		}
		t.Logf("Confirmation took %f seconds\n", stop.Sub(start).Seconds())
		require.Nil(t, err)
	})

}
