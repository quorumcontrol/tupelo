package actors

import (
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeSystem struct {
	actors []*actor.PID
}

func (fs *fakeSystem) GetRandomSyncer() *actor.PID {
	if len(fs.actors) == 0 {
		return nil
	}

	return fs.actors[rand.Intn(len(fs.actors))]
}

type fakeValidator struct {
	subscriptions []*actor.PID
}

func (fv *fakeValidator) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.SubscribeValidatorWorking:
		fv.subscriptions = append(fv.subscriptions, msg.Actor)
	}
}

func (fv *fakeValidator) notifyWorking() {
	for _, act := range fv.subscriptions {
		act.Tell(&messages.ValidatorWorking{})
	}
}

func (fv *fakeValidator) notifyClear() {
	for _, act := range fv.subscriptions {
		act.Tell(&messages.ValidatorClear{})
	}
}

func TestGossiper(t *testing.T) {
	var pusherMsgs []interface{}
	var currentPusher *actor.PID
	fakePusher := func(context actor.Context) {
		pusherMsgs = append(pusherMsgs, context.Message())
		currentPusher = context.Self()
	}

	pusherProps := actor.FromFunc(fakePusher)

	validator := &fakeValidator{}
	storage := actor.Spawn(actor.FromFunc(validator.Receive))
	defer storage.Poison()

	system := new(fakeSystem)

	gossiper := actor.Spawn(NewGossiperProps("test", storage, system, pusherProps))
	defer gossiper.Poison()

	gossiper.Tell(&messages.StartGossip{})
	time.Sleep(100 * time.Millisecond)
	assert.Len(t, pusherMsgs, 2)
	assert.IsType(t, pusherMsgs[0], &actor.Started{})
	assert.Equal(t, pusherMsgs[1].(*messages.DoPush).System, system)

	// doing another push should have no affect
	middleware.Log.Infow("do one gossip")
	gossiper.Tell(&messages.DoOneGossip{})

	assert.Len(t, pusherMsgs, 2)

	// if we terminate the pusher then it should gossip again
	middleware.Log.Infow("graceful stop")
	currentPusher.GracefulStop()
	time.Sleep(100 * time.Millisecond)
	assert.Len(t, pusherMsgs, 6) // 6 here because of stop, stopping, started, dopush being added

	// but if we have the validator working, we don't push
	middleware.Log.Infow("working")
	validator.notifyWorking()
	middleware.Log.Infow("graceful stop")
	currentPusher.GracefulStop()
	assert.Len(t, pusherMsgs, 8) // 8 here because of stop, stopping
	middleware.Log.Infow("notify clear")
	// but when the validator comes back online, we push again
	validator.notifyClear()
	time.Sleep(100 * time.Millisecond)
	assert.Len(t, pusherMsgs, 10) // 10 here because of started, dopush

	currentPusher.GracefulStop()
	validator.notifyWorking()

}

func TestFastGossip(t *testing.T) {
	system := new(fakeSystem)

	numNodes := 5
	nodes := make([]*actor.PID, numNodes, numNodes)
	stores := make([]*actor.PID, numNodes, numNodes)
	for i := 0; i < numNodes; i++ {
		storage := actor.Spawn(NewStorageProps())
		stores[i] = storage
		defer storage.Poison()
		pusherProps := NewPushSyncerProps("test", storage)
		gossiper, err := actor.SpawnPrefix(NewGossiperProps("test", storage, system, pusherProps), "g")
		require.Nil(t, err)
		defer gossiper.Poison()
		nodes[i] = gossiper
	}
	system.actors = nodes

	for _, node := range nodes {
		node.Tell(&messages.StartGossip{})
	}

	for i := 0; i < 1000; i++ {
		value := []byte(strconv.Itoa(i))
		key := crypto.Keccak256(value)
		nodes[rand.Intn(len(nodes))].Tell(&messages.Store{
			Key:   key,
			Value: value,
		})
	}

	value := []byte("hi")
	key := crypto.Keccak256(value)

	stores[0].Tell(&messages.Store{
		Key:   key,
		Value: value,
	})

	time.Sleep(100 * time.Millisecond)

	// assert all nodes know last transaction added
	for _, store := range stores {
		val, err := store.RequestFuture(&messages.Get{
			Key: key,
		}, 1*time.Second).Result()

		require.Nil(t, err)
		assert.Equal(t, value, val)
	}

}
