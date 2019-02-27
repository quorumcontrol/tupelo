package actors

import (
	"bytes"
	"math/rand"
	"strconv"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
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
	// var currentPusher *actor.PID
	fakePusher := func(context actor.Context) {
		pusherMsgs = append(pusherMsgs, context.Message())
		// currentPusher = context.Self()
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
}

func TestFastGossip(t *testing.T) {
	system := new(fakeSystem)

	numNodes := 5
	nodes := make([]*actor.PID, numNodes, numNodes)
	stores := make([]*actor.PID, numNodes, numNodes)
	for i := 0; i < numNodes; i++ {
		storage := actor.Spawn(NewStorageProps(storage.NewMemStorage()))
		stores[i] = storage
		defer storage.Poison()
		pusherProps := NewPushSyncerProps("test", storage)
		gossiper, err := actor.SpawnPrefix(NewGossiperProps("test", storage, system, pusherProps), "g")
		require.Nil(t, err)
		defer gossiper.Poison()
		nodes[i] = gossiper
	}
	system.actors = nodes

	// run 1000 transactions in before the one we check
	// just to make sure it doesn't clog up the system
	for i := 0; i < 1000; i++ {
		value := []byte(strconv.Itoa(i))
		key := crypto.Keccak256(value)
		stores[rand.Intn(len(stores))].Tell(&messages.Store{
			Key:   key,
			Value: value,
		})
	}
	for _, node := range nodes {
		node.Tell(&messages.StartGossip{})
	}

	value := []byte("hi")
	key := crypto.Keccak256(value)

	var actorsToKill []*actor.PID
	subscribe := func(key []byte, store *actor.PID) *actor.Future {
		fut := actor.NewFuture(1 * time.Second)
		subActor := func(context actor.Context) {
			switch msg := context.Message().(type) {
			case *messages.Store:
				if bytes.Equal(msg.Key, key) {
					fut.PID().Tell(msg)
				}
			}
		}
		act := actor.Spawn(actor.FromFunc(subActor))
		store.Tell(&messages.Subscribe{Subscriber: act})

		actorsToKill = append(actorsToKill, act)
		return fut
	}

	futures := make([]*actor.Future, len(stores)-1, len(stores)-1)
	for i := 1; i < len(stores); i++ {
		futures[i-1] = subscribe(key, stores[i])
	}
	defer func() {
		for _, act := range actorsToKill {
			act.Poison()
		}
	}()

	stores[0].Tell(&messages.Store{
		Key:   key,
		Value: value,
	})

	for _, future := range futures {
		res, err := future.Result()
		require.Nil(t, err)
		assert.Equal(t, key, res.(*messages.Store).Key)
	}
}
