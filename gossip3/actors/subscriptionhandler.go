package actors

import (
	"fmt"
	"strings"
	"time"

	"github.com/ipfs/go-cid"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
)

type actorPIDHolder map[string]*actor.PID
type subscriptionMap map[string]actorPIDHolder

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

	subscriptionManagers subscriptionMap
}

func NewSubscriptionHandlerProps() *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &SubscriptionHandler{
			subscriptionManagers: make(subscriptionMap),
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func subscriptionKeys(objectKey string, tip []byte) []string {
	if len(tip) == 0 {
		return []string{objectKey, ""}
	}

	tipCid, err := cid.Cast(tip)
	if err != nil {
		panic(fmt.Errorf("error casting new tip to CID: %v", err))
	}

	return []string{objectKey, tipCid.String()}
}

func (sh *SubscriptionHandler) subscriptionKeys(uncastMsg interface{}) []string {
	switch msg := uncastMsg.(type) {
	case *extmsgs.TipSubscription:
		return subscriptionKeys(string(msg.ObjectID), msg.TipValue)
	case *messages.CurrentStateWrapper:
		return subscriptionKeys(string(msg.CurrentState.Signature.ObjectID), msg.CurrentState.Signature.NewTip)
	case *extmsgs.Error:
		return subscriptionKeys(msg.Source, make([]byte, 0))
	default:
		return nil
	}
}

func (sh *SubscriptionHandler) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Terminated:
		split := strings.Split(msg.Who.GetId(), "/")
		objectID := split[len(split)-1]
		if _, ok := sh.subscriptionManagers[objectID]; ok {
			delete(sh.subscriptionManagers, objectID)
		}
	case *extmsgs.TipSubscription:
		subKeys := sh.subscriptionKeys(msg)
		var manager *actor.PID
		objManagers, ok := sh.subscriptionManagers[subKeys[0]]
		if ok {
			manager, ok = objManagers[subKeys[1]]
			if !ok {
				if msg.Unsubscribe {
					return
				}
				manager = sh.newManager(context, msg)
			}
		} else {
			if msg.Unsubscribe {
				return
			}
			manager = sh.newManager(context, msg)
		}

		context.Forward(manager)
	case *messages.CurrentStateWrapper, *extmsgs.Error:
		subKeys := sh.subscriptionKeys(msg)
		var managers = make([]*actor.PID, 0)
		if len(subKeys[1]) == 0 {
			// send to all subscribers of this objectID
			managersMap, ok := sh.subscriptionManagers[subKeys[0]]
			if ok {
				for _, m := range managersMap {
					managers = append(managers, m)
				}
			}
		} else {
			// just send to subscribers for this tip value
			manager, ok := sh.subscriptionManagers[subKeys[0]][subKeys[1]]
			if ok {
				managers = append(managers, manager)
			}
		}

		for _, m := range managers {
			context.Forward(m)
		}
	}
}

func (sh *SubscriptionHandler) newManager(context actor.Context, msg *extmsgs.TipSubscription) *actor.PID {
	subKeys := sh.subscriptionKeys(msg)
	actorNameKeys := subKeys
	if len(subKeys[1]) == 0 {
		actorNameKeys[1] = "all"
	}
	m, err := context.SpawnNamed(newObjectSubscriptionManagerProps(), strings.Join(actorNameKeys, "-"))
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}
	if sh.subscriptionManagers[subKeys[0]] == nil {
		sh.subscriptionManagers[subKeys[0]] = make(actorPIDHolder)
	}
	sh.subscriptionManagers[subKeys[0]][subKeys[1]] = m
	return m
}

func (osm *objectSubscriptionManager) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.ReceiveTimeout:
		context.SetReceiveTimeout(0)
		osm.Log.Debugw("killing unused subscription")
		context.Self().Stop()
	case *extmsgs.TipSubscription:
		context.SetReceiveTimeout(subscriptionTimeout)
		if msg.Unsubscribe {
			osm.unsubscribe(context, msg)
		} else {
			osm.subscribe(context, msg)
		}
	case *messages.CurrentStateWrapper, *extmsgs.Error:
		context.SetReceiveTimeout(subscriptionTimeout)
		osm.notifySubscribers(context, msg)
	}
}

func (osm *objectSubscriptionManager) subscriptionKeys(context actor.Context, msg *extmsgs.TipSubscription) []string {
	return subscriptionKeys(context.Sender().String(), msg.TipValue)
}

func (osm *objectSubscriptionManager) subscribe(context actor.Context, msg *extmsgs.TipSubscription) {
	osm.subscriptions[context.Sender().String()] = context.Sender()
}

func (osm *objectSubscriptionManager) unsubscribe(context actor.Context, msg *extmsgs.TipSubscription) {
	delete(osm.subscriptions, context.Sender().String())
	if len(osm.subscriptions) == 0 {
		context.Self().Stop()
	}
}

func (osm *objectSubscriptionManager) notifySubscribers(context actor.Context, uncastMsg interface{}) {
	var notice interface{}
	switch msg := uncastMsg.(type) {
	case *messages.CurrentStateWrapper:
		notice = msg.CurrentState
	case *extmsgs.Error:
		notice = msg
	}
	for _, sub := range osm.subscriptions {
		context.Request(sub, notice)
	}
}
