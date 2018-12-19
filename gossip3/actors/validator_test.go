package actors

import (
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/stretchr/testify/require"
)

func TestValidator(t *testing.T) {
	currentState := actor.Spawn(NewStorageProps())
	defer currentState.Poison()
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

	trans := newValidTransaction(t)
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
