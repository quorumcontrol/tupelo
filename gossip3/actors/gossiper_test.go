package actors

import (
	"math/rand"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/stretchr/testify/assert"
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
