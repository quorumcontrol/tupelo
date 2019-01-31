package actors

import (
	"fmt"
	"github.com/ipfs/go-cid"
	"strings"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
)

type actorPIDHolder map[string]*actor.PID

var subscriptionTimeout = 1 * time.Minute

type objectSubscriptionManager struct {
	middleware.LogAwareHolder

	subscriptions actorPIDHolder
}

func newObjectSubscriptionManagerProps() *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &objectSubscriptionManager{
			subscriptions: make(actorPIDHolder),
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

// The SubscriptionHandler is responsible for informing actors on tip changes. It is implemented
// using workers for each ObjectID because it's easier to implement a timeout that way.
// Any subscription that is not notified within subscriptionTimeout duration will automatically
// deregister itself.
type SubscriptionHandler struct {
	middleware.LogAwareHolder

	subscriptionManagers actorPIDHolder
}

func NewSubscriptionHandlerProps() *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &SubscriptionHandler{
			subscriptionManagers: make(actorPIDHolder),
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func tipSubscriptionKey(uncastMsg interface{}) string {
	switch msg := uncastMsg.(type) {
	case *messages.TipSubscription:
		return string(msg.ObjectID)+"-"+msg.TipValue
	case *messages.CurrentStateWrapper:
		newTip, err := cid.Cast(msg.CurrentState.Signature.NewTip)
		if err != nil {
			panic(fmt.Errorf("error casting new tip to CID: %v", err))
		}
		return string(msg.CurrentState.Signature.ObjectID)+"-"+newTip.String()
	}
	return "" // shouldn't get here; seems nicer than having to deal with an error
}

func (sh *SubscriptionHandler) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Terminated:
		split := strings.Split(msg.Who.GetId(), "/")
		objectID := split[len(split)-1]
		if _, ok := sh.subscriptionManagers[objectID]; ok {
			delete(sh.subscriptionManagers, objectID)
		}
	case *messages.TipSubscription:
		fmt.Printf("sh got tip sub for %s\n", tipSubscriptionKey(msg))
		manager, ok := sh.subscriptionManagers[tipSubscriptionKey(msg)]
		if msg.Unsubscribe && !ok {
			return
		}
		if !ok {
			manager = sh.newManager(context, msg)
		}

		context.Forward(manager)
	case *messages.CurrentStateWrapper:
		fmt.Printf("sh got current state change for %s\n", tipSubscriptionKey(msg))
		manager, ok := sh.subscriptionManagers[tipSubscriptionKey(msg)]
		if ok {
			fmt.Println("found sub manager to forward to")
			context.Forward(manager)
		}
	}
}

func (sh *SubscriptionHandler) newManager(context actor.Context, msg *messages.TipSubscription) *actor.PID {
	tsk := tipSubscriptionKey(msg)
	m, err := context.SpawnNamed(newObjectSubscriptionManagerProps(), tsk)
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}
	sh.subscriptionManagers[tsk] = m
	return m
}

func (osm *objectSubscriptionManager) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.ReceiveTimeout:
		context.SetReceiveTimeout(0)
		osm.Log.Debugw("killing unused subscription")
		context.Self().Stop()
	case *messages.TipSubscription:
		context.SetReceiveTimeout(subscriptionTimeout)
		if msg.Unsubscribe {
			osm.unsubscribe(context, msg)
		} else {
			osm.subscribe(context, msg)
		}
	case *messages.CurrentStateWrapper:
		context.SetReceiveTimeout(subscriptionTimeout)
		osm.notifySubscribers(context, msg)
	}
}

func (osm *objectSubscriptionManager) subscribe(context actor.Context, msg *messages.TipSubscription) {
	osm.subscriptions[context.Sender().String()+"-"+msg.TipValue] = context.Sender()
}

func (osm *objectSubscriptionManager) unsubscribe(context actor.Context, msg *messages.TipSubscription) {
	delete(osm.subscriptions, context.Sender().String()+"-"+msg.TipValue)
	if len(osm.subscriptions) == 0 {
		context.Self().Stop()
	}
}

func (osm *objectSubscriptionManager) notifySubscribers(context actor.Context, msg *messages.CurrentStateWrapper) {
	for _, sub := range osm.subscriptions {
		//tipValue := strings.Split(key, "-")[1]
		//newTip, err := cid.Cast(msg.CurrentState.Signature.NewTip)
		//if err != nil {
		//	panic(fmt.Errorf("error casting new tip to CID: %v", err))
		//}
		//if newTip.String() == tipValue {
		//	context.Request(sub, msg.CurrentState)
		//}
		context.Request(sub, msg.CurrentState)
	}
}
