package gossip3

import (
	"bytes"
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo/gossip3/actors"
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
	numMembers := 10
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

	trans := newValidTransaction(t)
	bits, err := trans.MarshalMsg(nil)
	require.Nil(t, err)
	id := crypto.Keccak256(bits)

	key := id
	middleware.Log.Infow("tests", "key", key, "objID", trans.ObjectID, "cs", string(append(trans.ObjectID, trans.PreviousTip...)))
	value := bits

	for _, s := range syncers {
		s.Actor.Tell(&messages.StartGossip{})
	}
	start := time.Now()

	syncers[0].Actor.Tell(&messages.Store{
		Key:   key,
		Value: value,
	})

	var stop time.Time
	for {
		if (time.Now().Sub(start)) > (5 * time.Second) {
			t.Fatal("timed out looking for done function")
			break
		}
		val, err := syncers[0].Actor.RequestFuture(&messages.GetTip{ObjectID: trans.ObjectID}, 1*time.Second).Result()
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
		time.Sleep(100 * time.Millisecond)
	}

	t.Logf("Confirmation took %f seconds\n", stop.Sub(start).Seconds())
	assert.True(t, stop.Sub(start) < 60*time.Second)

	// This bit of commented out code will run the CPU profiler
	// f, ferr := os.Create("../gossip.prof")
	// if ferr != nil {
	// 	t.Fatal(ferr)
	// }
	// pprof.StartCPUProfile(f)
	// defer pprof.StopCPUProfile()

}
