package gossip3

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/tupelo/gossip3/actors"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/stretchr/testify/require"
)

func newTupeloSystem(ctx context.Context, count int) (*System, error) {
	system := NewSystem()
	for i := 0; i < count; i++ {
		syncer, err := actor.SpawnPrefix(actors.NewTupeloNodeProps(), "tupelo")
		if err != nil {
			return nil, fmt.Errorf("error spawning: %v", err)
		}
		go func() {
			<-ctx.Done()
			syncer.Stop()
		}()
		system.Syncers.Add(syncer)
	}
	return system, nil
}

func TestTupeloMemStorage(t *testing.T) {
	numMembers := 3
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	system, err := newTupeloSystem(ctx, numMembers)
	require.Nil(t, err)

	syncers := system.Syncers.Values()
	require.Len(t, syncers, numMembers)
	t.Logf("syncers: %v", syncers)
	syncer := syncers[0]

	key := []byte("hi this is a long key that does many things")
	value := []byte("hiyo")

	syncer.Tell(&messages.Store{
		Key:   key,
		Value: value,
	})

	time.Sleep(10 * time.Millisecond)

	val, err := syncer.RequestFuture(&messages.Get{Key: key}, 1*time.Second).Result()
	require.Nil(t, err)
	require.Equal(t, value, val)
}

func TestTupeloGossip(t *testing.T) {
	numMembers := 200
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	system, err := newTupeloSystem(ctx, numMembers)
	require.Nil(t, err)

	syncers := system.Syncers.Values()
	require.Len(t, syncers, numMembers)

	wg := sync.WaitGroup{}
	wg.Add(numMembers)

	key := []byte("hi this is a long key that does many things")
	value := []byte("hiyo")

	syncers[0].Tell(&messages.Store{
		Key:   key,
		Value: value,
	})
	syncers[1].Tell(&messages.Store{
		Key:   key,
		Value: value,
	})
	syncers[2].Tell(&messages.Store{
		Key:   key,
		Value: value,
	})

	for _, s := range syncers {
		s.Tell(&messages.StartGossip{
			System: system,
		})
		b := s
		go func(syncer *actor.PID) {
			val, err := syncer.RequestFuture(&messages.Get{Key: key}, 1*time.Second).Result()
			require.Nil(t, err)

			timer := time.AfterFunc(2*time.Second, func() {
				t.Logf("TIMEOUT %s", syncer.GetId())
				wg.Done()
				t.Fatalf("timeout waiting for key to appear")
			})
			for !bytes.Equal(val.([]byte), value) {
				time.Sleep(1 * time.Second)
				val, err = syncer.RequestFuture(&messages.Get{Key: key}, 2*time.Second).Result()
				require.Nil(t, err)
			}
			require.Equal(t, value, val.([]byte))
			timer.Stop()
			wg.Done()
		}(&b)
	}
	wg.Wait()
}
