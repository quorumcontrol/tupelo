package actors

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/tupelo/storage"

	"github.com/quorumcontrol/messages/build/go/signatures"
	"github.com/quorumcontrol/tupelo-go-sdk/client"
	"github.com/quorumcontrol/tupelo-go-sdk/consensus"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/testhelpers"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTupeloSystem(ctx context.Context, testSet *testnotarygroup.TestSet) (*types.NotaryGroup, remote.PubSub, error) {
	simulatedPubSub := remote.NewSimulatedPubSub()

	ng := types.NewNotaryGroup("testnotary")
	for i, signKey := range testSet.SignKeys {
		sk := signKey
		signer := types.NewLocalSigner(testSet.PubKeys[i], sk)
		syncer, err := actor.EmptyRootContext.SpawnNamed(NewTupeloNodeProps(&TupeloConfig{
			Self:              signer,
			NotaryGroup:       ng,
			CurrentStateStore: storage.NewDefaultMemory(),
			PubSubSystem:      simulatedPubSub,
		}), "tupelo-"+signer.ID)
		if err != nil {
			return nil, nil, fmt.Errorf("error spawning: %v", err)
		}
		signer.Actor = syncer
		go func() {
			<-ctx.Done()
			actor.EmptyRootContext.Poison(syncer)
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
		cli := client.New(system, string(trans.ObjectId), pubsub)
		err := cli.SendTransaction(&trans)
		require.Nil(t, err)
	}

	t.Run("commits a good transaction", func(t *testing.T) {
		trans := testhelpers.NewValidTransaction(t)
		t.Logf("trans id: %s, ObjectId: %s, base64 obj: %s", base64.StdEncoding.EncodeToString(consensus.RequestID(&trans)), string(trans.ObjectId), base64.StdEncoding.EncodeToString(trans.ObjectId))

		cli := client.New(system, string(trans.ObjectId), pubsub)
		cli.Listen()
		defer cli.Stop()

		fut := cli.Subscribe(&trans, 10*time.Second)

		err := cli.SendTransaction(&trans)
		require.Nil(t, err)

		resp, err := fut.Result()

		require.Nil(t, err)
		assert.Equal(t, resp.(*signatures.CurrentState).Signature.NewTip, trans.NewTip)
		assert.Equal(t, resp.(*signatures.CurrentState).Signature.ObjectId, trans.ObjectId)
	})

}
