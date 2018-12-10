package gossip3

import (
	"fmt"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/tupelo/gossip3/actors"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newSystem(count int) (*System, error) {
	system := NewSystem()
	for i := 0; i < count; i++ {
		syncer, err := actor.SpawnPrefix(actors.GossiperProps, "gossiper")
		if err != nil {
			return nil, fmt.Errorf("error spawning: %v", err)
		}
		system.Syncers.Add(syncer)
	}
	return system, nil
}

func TestStart(t *testing.T) {
	system, err := newSystem(3)
	require.Nil(t, err)
	syncer := system.GetRandomSyncer()
	syncer.Tell(&messages.Store{
		Key:   []byte("hi this is a long key that does many things"),
		Value: []byte("hiyo"),
	})

	syncer.Tell(&messages.StartGossip{
		System: system,
	})
	assert.Len(t, system.Syncers.Values(), 3)
	time.Sleep(2 * time.Second)
}
