package actors

import (
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/testhelpers"
	"github.com/stretchr/testify/require"
)

func TestValidator(t *testing.T) {
	currentState := storage.NewMemStorage()
	validator := actor.Spawn(NewTransactionValidatorProps(currentState))
	defer validator.Poison()

	fut := actor.NewFuture(1 * time.Second)
	validatorSenderFunc := func(context actor.Context) {
		switch msg := context.Message().(type) {
		case *messages.Store:
			context.Request(validator, msg)
		case *messages.TransactionWrapper:
			fut.PID().Tell(msg)
		}
	}

	sender := actor.Spawn(actor.FromFunc(validatorSenderFunc))
	defer sender.Poison()

	trans := testhelpers.NewValidTransaction(t)
	value, err := trans.MarshalMsg(nil)
	require.Nil(t, err)
	key := crypto.Keccak256(value)

	sender.Tell(&messages.Store{
		Key:   key,
		Value: value,
	})

	_, err = fut.Result()
	require.Nil(t, err)
}

func BenchmarkValidator(b *testing.B) {
	currentState := storage.NewMemStorage()
	validator := actor.Spawn(NewTransactionValidatorProps(currentState))
	defer validator.Poison()

	trans := testhelpers.NewValidTransaction(b)
	value, err := trans.MarshalMsg(nil)
	require.Nil(b, err)
	key := crypto.Keccak256(value)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := validator.RequestFuture(&messages.Store{
			Key:   key,
			Value: value,
		}, 1*time.Second).Result()
		require.Nil(b, err)
	}
}
