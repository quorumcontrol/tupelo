package actors

import (
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/differencedigest/ibf"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImmutableIBF(t *testing.T) {
	s := actor.Spawn(NewStorageProps(storage.NewMemStorage()))
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

func TestSetupIBFAtStart(t *testing.T) {
	store := storage.NewMemStorage()
	key := []byte("12345678910")
	err := store.Set(key, []byte("hi"))
	require.Nil(t, err)
	testIBF := ibf.NewInvertibleBloomFilter(500, 4)
	testIBF.Add(byteToIBFsObjectId(key[0:8]))

	s := actor.Spawn(NewStorageProps(store))
	defer s.Poison()

	ibfInter, err := s.RequestFuture(&messages.GetIBF{Size: 500}, 1*time.Second).Result()
	require.Nil(t, err)
	actorIBF := ibfInter.(*ibf.InvertibleBloomFilter)
	assert.Equal(t, testIBF, actorIBF)
}

func TestSubscription(t *testing.T) {
	s := actor.Spawn(NewStorageProps(storage.NewMemStorage()))
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
