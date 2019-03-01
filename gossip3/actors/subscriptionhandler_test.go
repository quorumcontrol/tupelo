package actors

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo-go-client/consensus"

	"github.com/AsynkronIT/protoactor-go/actor"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
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

	s.Request(&extmsgs.TipSubscription{
		ObjectID: objectID,
		TipValue: tipValue.Bytes(),
	}, fut.PID())

	currentState := &extmsgs.CurrentState{
		Signature: &extmsgs.Signature{
			ObjectID: objectID,
			NewTip:   tipValue.Bytes(),
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

	s.Request(&extmsgs.TipSubscription{
		ObjectID: objectID,
		TipValue: tipValue.Bytes(),
	}, fut.PID())

	s.Request(&extmsgs.TipSubscription{
		Unsubscribe: true,
		ObjectID:    objectID,
		TipValue:    tipValue.Bytes(),
	}, fut.PID())

	currentState := &extmsgs.CurrentState{
		Signature: &extmsgs.Signature{
			ObjectID: objectID,
			NewTip:   tipValue.Bytes(),
		},
	}

	s.Tell(&messages.CurrentStateWrapper{
		CurrentState: currentState,
	})

	_, err = fut.Result()
	require.NotNil(t, err)
}
