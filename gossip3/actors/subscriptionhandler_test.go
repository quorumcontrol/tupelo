package actors

import (
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscribe(t *testing.T) {
	s := actor.Spawn(NewSubscriptionHandlerProps())
	defer s.GracefulStop()

	fut := actor.NewFuture(100 * time.Millisecond)

	objectID := []byte("afakeobjectidjustfortesting")

	s.Request(&messages.TipSubscription{
		ObjectID: objectID,
	}, fut.PID())

	currentState := &messages.CurrentState{
		Signature: &messages.Signature{
			ObjectID: objectID,
		},
	}

	s.Tell(&messages.CurrentStateWrapper{
		CurrentState: currentState,
	})

	received, err := fut.Result()
	require.Nil(t, err)
	assert.Equal(t, currentState, received)
}

func TestUnsubscribe(t *testing.T) {
	s := actor.Spawn(NewSubscriptionHandlerProps())
	defer s.GracefulStop()

	fut := actor.NewFuture(100 * time.Millisecond)

	objectID := []byte("afakeobjectidjustfortesting")

	s.Request(&messages.TipSubscription{
		ObjectID: objectID,
	}, fut.PID())

	s.Request(&messages.TipSubscription{
		Unsubscribe: true,
		ObjectID:    objectID,
	}, fut.PID())

	currentState := &messages.CurrentState{
		Signature: &messages.Signature{
			ObjectID: objectID,
		},
	}

	s.Tell(&messages.CurrentStateWrapper{
		CurrentState: currentState,
	})

	_, err := fut.Result()
	require.NotNil(t, err)
}
