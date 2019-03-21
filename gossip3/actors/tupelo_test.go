package actors

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/storage"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/testhelpers"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/types"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTupeloSystem(ctx context.Context, testSet *testnotarygroup.TestSet) (*types.NotaryGroup, error) {
	ng := types.NewNotaryGroup("testnotary")
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

	rootContext := actor.EmptyRootContext

	for i := 0; i < 100; i++ {
		trans := testhelpers.NewValidTransaction(t)
		bits, err := trans.MarshalMsg(nil)
		require.Nil(t, err)
		key := crypto.Keccak256(bits)
		rootContext.Send(syncers[rand.Intn(len(syncers))].Actor, &extmsgs.Store{
			Key:   key,
			Value: bits,
		})
		rootContext.Send(syncers[rand.Intn(len(syncers))].Actor, &extmsgs.Store{
			Key:   key,
			Value: bits,
		})
		rootContext.Send(syncers[rand.Intn(len(syncers))].Actor, &extmsgs.Store{
			Key:   key,
			Value: bits,
		})
		if err != nil {
			t.Fatalf("error sending transaction: %v", err)
		}
	}

	for _, s := range syncers {
		rootContext.Send(s.Actor, &messages.StartGossip{})
	}

	t.Run("commits a good transaction", func(t *testing.T) {
		trans := testhelpers.NewValidTransaction(t)
		bits, err := trans.MarshalMsg(nil)
		require.Nil(t, err)
		key := crypto.Keccak256(bits)

		fut := actor.NewFuture(20 * time.Second)

		syncer := syncers[rand.Intn(len(syncers))].Actor

		newTip, _ := cid.Cast(trans.NewTip)
		rootContext.RequestWithCustomSender(syncer, &extmsgs.TipSubscription{
			ObjectID: trans.ObjectID,
			TipValue: newTip.Bytes(),
		}, fut.PID())

		rootContext.Send(syncer, &extmsgs.Store{
			Key:   key,
			Value: bits,
		})

		resp, err := fut.Result()
		require.Nil(t, err)
		assert.Equal(t, resp.(*extmsgs.CurrentState).Signature.NewTip, trans.NewTip)
	})

	t.Run("reaches another node", func(t *testing.T) {
		trans := testhelpers.NewValidTransaction(t)
		bits, err := trans.MarshalMsg(nil)
		require.Nil(t, err)
		key := crypto.Keccak256(bits)

		fut := actor.NewFuture(20 * time.Second)

		newTip, _ := cid.Cast(trans.NewTip)
		rootContext.RequestWithCustomSender(syncers[1].Actor, &extmsgs.TipSubscription{
			ObjectID: trans.ObjectID,
			TipValue: newTip.Bytes(),
		}, fut.PID())

		start := time.Now()
		rootContext.Send(syncers[0].Actor, &extmsgs.Store{
			Key:   key,
			Value: bits,
		})

		resp, err := fut.Result()
		require.Nil(t, err)
		assert.Equal(t, resp.(*extmsgs.CurrentState).Signature.NewTip, trans.NewTip)

		tipFut := actor.NewFuture(5 * time.Second)
		rootContext.RequestWithCustomSender(syncers[1].Actor, &extmsgs.GetTip{
			ObjectID: []byte(trans.ObjectID),
		}, tipFut.PID())
		tipResp, err := tipFut.Result()
		assert.Equal(t, tipResp.(*extmsgs.CurrentState).Signature.NewTip, trans.NewTip)

		stop := time.Now()
		t.Logf("Confirmation took %f seconds\n", stop.Sub(start).Seconds())
	})

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

	trans := testhelpers.NewValidTransaction(t)
	bits, err := trans.MarshalMsg(nil)
	require.Nil(t, err)
	id := crypto.Keccak256(bits)

	key := id
	value := bits

	rootContext := actor.EmptyRootContext

	rootContext.Send(syncer, &extmsgs.Store{
		Key:   key,
		Value: value,
	})

	time.Sleep(10 * time.Millisecond)

	val, err := rootContext.RequestFuture(syncer, &messages.Get{Key: key}, 1*time.Second).Result()
	require.Nil(t, err)
	require.Equal(t, value, val)
}
