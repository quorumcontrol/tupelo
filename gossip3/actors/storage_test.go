package actors

import (
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/differencedigest/ibf"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImmutableIBF(t *testing.T) {
	s := actor.Spawn(NewStorageProps())
	defer s.Poison()

	value := []byte("hi")
	key := crypto.Keccak256(value)

	s.Tell(&messages.Store{Key: key, Value: value})
	ibfInter, err := s.RequestFuture(&messages.GetIBF{Size: 500}, 1*time.Second).Result()
	require.Nil(t, err)
	ibf1 := ibfInter.(*ibf.InvertibleBloomFilter)

	value2 := []byte("hi2")
	key2 := crypto.Keccak256(value2)

	s.Tell(&messages.Store{Key: key2, Value: value2})

	time.Sleep(100 * time.Millisecond)

	ibfInter, err = s.RequestFuture(&messages.GetIBF{Size: 500}, 1*time.Second).Result()
	require.Nil(t, err)
	ibf2 := ibfInter.(*ibf.InvertibleBloomFilter)

	assert.NotEqual(t, ibf1, ibf2)
}

func TestSubscription(t *testing.T) {
	s := actor.Spawn(NewStorageProps())
	defer s.Poison()

	var msgs []interface{}
	subscriber := func(context actor.Context) {
		msgs = append(msgs, context.Message())
	}

	sub := actor.Spawn(actor.FromFunc(subscriber))
	defer sub.Poison()

	value := []byte("hi")
	key := crypto.Keccak256(value)

	s.Tell(&messages.Store{Key: key, Value: value})
	time.Sleep(100 * time.Millisecond)

	require.Len(t, msgs, 1) // only the actor started

	s.Tell(&messages.Subscribe{Subscriber: sub})
	s.Tell(&messages.Store{Key: key, Value: value})
	time.Sleep(100 * time.Millisecond)

	require.Len(t, msgs, 1)

	value = []byte("hi2")
	key = crypto.Keccak256(value)

	s.Tell(&messages.Store{Key: key, Value: value})
	time.Sleep(100 * time.Millisecond)
	require.Len(t, msgs, 2)
	require.Equal(t, &messages.Store{Key: key, Value: value}, msgs[len(msgs)-1])
}
