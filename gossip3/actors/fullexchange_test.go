package actors

import (
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/storage"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/stretchr/testify/require"
)

func TestFullExchange(t *testing.T) {
	rootContext := actor.EmptyRootContext
	storeA := storage.NewMemStorage()
	storageA := rootContext.Spawn(NewStorageProps(storeA))
	defer storageA.Poison()
	storeB := storage.NewMemStorage()
	storageB := rootContext.Spawn(NewStorageProps(storeB))
	defer storageB.Poison()

	for i := 1; i <= 5; i++ {
		val := crypto.Keccak256([]byte{byte(i)})
		key := crypto.Keccak256(val)
		rootContext.Request(storageA, &extmsgs.Store{
			Key:        key,
			Value:      val,
			SkipNotify: true,
		})
	}

	for i := 6; i <= 10; i++ {
		val := crypto.Keccak256([]byte{byte(i)})
		key := crypto.Keccak256(val)
		rootContext.Request(storageB, &extmsgs.Store{
			Key:        key,
			Value:      val,
			SkipNotify: true,
		})
	}

	exchangeA := rootContext.Spawn(NewFullExchangeProps(storageA))
	defer exchangeA.Poison()
	exchangeB := rootContext.Spawn(NewFullExchangeProps(storageB))
	defer exchangeB.Poison()

	rootContext.RequestFuture(exchangeA, &messages.RequestFullExchange{
		DestinationHolder: messages.DestinationHolder{
			Destination: extmsgs.ToActorPid(exchangeB),
		},
	}, 2*time.Second).Wait()

	// Give time for Store messages to go through
	time.Sleep(100 * time.Millisecond)

	count := 0

	storeA.ForEach([]byte{}, func(key, value []byte) error {
		count++
		targetStoreValue, err := storeB.Get(key)
		require.Nil(t, err)
		require.Equal(t, value, targetStoreValue)
		return nil
	})

	require.Equal(t, count, 10)
}
