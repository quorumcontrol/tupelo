package actors

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo-go-client/client"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/remote"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/testhelpers"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/types"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTupeloSystem(ctx context.Context, testSet *testnotarygroup.TestSet) (*types.NotaryGroup, remote.PubSub, error) {
	simulatedPubSub := remote.NewSimulatedPubSub()

	ng := types.NewNotaryGroup("testnotary")
	for i, signKey := range testSet.SignKeys {
		sk := signKey
		signer := types.NewLocalSigner(testSet.PubKeys[i].ToEcdsaPub(), sk)
		syncer, err := actor.EmptyRootContext.SpawnNamed(NewTupeloNodeProps(&TupeloConfig{
			Self:              signer,
			NotaryGroup:       ng,
			CurrentStateStore: storage.NewMemStorage(),
			PubSubSystem:      simulatedPubSub,
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

	return ng, simulatedPubSub, nil
}

func TestCommits(t *testing.T) {
	numMembers := 3
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		middleware.Log.Infow("---- tests over ----")
		cancel()
	}()
	ts := testnotarygroup.NewTestSet(t, numMembers)

	system, pubsub, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)

	syncers := system.AllSigners()
	require.Len(t, system.Signers, numMembers)
	t.Logf("syncer 0 id: %s", syncers[0].ID)

	for i := 0; i < 100; i++ {
		trans := testhelpers.NewValidTransaction(t)
		cli := client.New(system, string(trans.ObjectID), pubsub)
		err := cli.SendTransaction(&trans)
		require.Nil(t, err)
	}

	t.Run("commits a good transaction", func(t *testing.T) {
		trans := testhelpers.NewValidTransaction(t)

		cli := client.New(system, string(trans.ObjectID), pubsub)
		cli.Listen()
		defer cli.Stop()

		fut := cli.Subscribe(&trans, 10*time.Second)

		err := cli.SendTransaction(&trans)
		require.Nil(t, err)

		resp, err := fut.Result()

		require.Nil(t, err)
		assert.Equal(t, resp.(*extmsgs.CurrentState).Signature.NewTip, trans.NewTip)
	})

}
