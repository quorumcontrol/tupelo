package actors

import (
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/consensus"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscribe(t *testing.T) {
	s := actor.Spawn(NewSubscriptionHandlerProps())
	defer s.GracefulStop()

	fut := actor.NewFuture(100 * time.Millisecond)

	objectID := []byte("afakeobjectidjustfortesting")

	treeKey, err := crypto.GenerateKey()
	require.Nil(t, err)
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())
	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	emptyTree := consensus.NewEmptyTree(treeDID, nodeStore)

	tipValue := emptyTree.Tip

	s.Request(&messages.TipSubscription{
		ObjectID: objectID,
		TipValue: tipValue.String(),
	}, fut.PID())

	currentState := &messages.CurrentState{
		Signature: &messages.Signature{
			ObjectID: objectID,
			NewTip: tipValue.Bytes(),
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

	treeKey, err := crypto.GenerateKey()
	require.Nil(t, err)
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())
	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	emptyTree := consensus.NewEmptyTree(treeDID, nodeStore)

	tipValue := emptyTree.Tip

	s.Request(&messages.TipSubscription{
		ObjectID: objectID,
		TipValue: tipValue.String(),
	}, fut.PID())

	s.Request(&messages.TipSubscription{
		Unsubscribe: true,
		ObjectID:    objectID,
		TipValue:    tipValue.String(),
	}, fut.PID())

	currentState := &messages.CurrentState{
		Signature: &messages.Signature{
			ObjectID: objectID,
			NewTip: tipValue.Bytes(),
		},
	}

	s.Tell(&messages.CurrentStateWrapper{
		CurrentState: currentState,
	})

	_, err = fut.Result()
	require.NotNil(t, err)
}
