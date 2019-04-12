package actors

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/storage"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/remote"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/testhelpers"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/types"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTupeloSystem(ctx context.Context, testSet *testnotarygroup.TestSet) (*types.NotaryGroup, *remote.SimulatedBroadcaster, error) {
	txType := (&extmsgs.Transaction{}).TypeCode()
	broadcaster := remote.NewSimulatedBroadcaster()
	ng := types.NewNotaryGroup("testnotary")
	for i, signKey := range testSet.SignKeys {
		sk := signKey
		signer := types.NewLocalSigner(testSet.PubKeys[i].ToEcdsaPub(), sk)
		syncer, err := actor.EmptyRootContext.SpawnNamed(NewTupeloNodeProps(&TupeloConfig{
			Self:                   signer,
			NotaryGroup:            ng,
			CommitStore:            storage.NewMemStorage(),
			CurrentStateStore:      storage.NewMemStorage(),
			BroadcastSubscriberProps: broadcaster.NewSubscriberProps(txType),
		}), "tupelo-"+signer.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("error spawning: %v", err)
		}
		signer.Actor = syncer
		go func() {
			<-ctx.Done()
			syncer.Poison()
		}()
		ng.AddSigner(signer)
	}
	return ng, broadcaster, nil
}

func TestCommits(t *testing.T) {
	numMembers := 20
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		middleware.Log.Infow("---- tests over ----")
		cancel()
	}()
	ts := testnotarygroup.NewTestSet(t, numMembers)

	system, broadcaster, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)

	syncers := system.AllSigners()
	require.Len(t, system.Signers, numMembers)
	t.Logf("syncer 0 id: %s", syncers[0].ID)

	rootContext := actor.EmptyRootContext

	for i := 0; i < 100; i++ {
		trans := testhelpers.NewValidTransaction(t)
		err := broadcaster.Broadcast(&trans)
		require.Nil(t, err)
	}

	for _, s := range syncers {
		rootContext.Send(s.Actor, &messages.StartGossip{})
	}

	t.Run("commits a good transaction", func(t *testing.T) {
		trans := testhelpers.NewValidTransaction(t)

		fut := actor.NewFuture(20 * time.Second)

		syncer := syncers[rand.Intn(len(syncers))].Actor

		newTip, _ := cid.Cast(trans.NewTip)
		rootContext.RequestWithCustomSender(syncer, &extmsgs.TipSubscription{
			ObjectID: trans.ObjectID,
			TipValue: newTip.Bytes(),
		}, fut.PID())

		err := broadcaster.Broadcast(&trans)
		require.Nil(t, err)

		resp, err := fut.Result()
		require.Nil(t, err)
		assert.Equal(t, resp.(*extmsgs.CurrentState).Signature.NewTip, trans.NewTip)
	})

	t.Run("reaches another node", func(t *testing.T) {
		trans := testhelpers.NewValidTransaction(t)

		fut := actor.NewFuture(20 * time.Second)

		newTip, _ := cid.Cast(trans.NewTip)
		rootContext.RequestWithCustomSender(syncers[1].Actor, &extmsgs.TipSubscription{
			ObjectID: trans.ObjectID,
			TipValue: newTip.Bytes(),
		}, fut.PID())

		start := time.Now()

		err := broadcaster.Broadcast(&trans)
		require.Nil(t, err)

		resp, err := fut.Result()
		require.Nil(t, err)
		assert.Equal(t, resp.(*extmsgs.CurrentState).Signature.NewTip, trans.NewTip)

		tipFut := actor.NewFuture(5 * time.Second)
		rootContext.RequestWithCustomSender(syncers[1].Actor, &extmsgs.GetTip{
			ObjectID: []byte(trans.ObjectID),
		}, tipFut.PID())
		tipResp, err := tipFut.Result()
		require.Nil(t, err)
		assert.Equal(t, tipResp.(*extmsgs.CurrentState).Signature.NewTip, trans.NewTip)

		stop := time.Now()
		t.Logf("Confirmation took %f seconds\n", stop.Sub(start).Seconds())
	})

}
