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
