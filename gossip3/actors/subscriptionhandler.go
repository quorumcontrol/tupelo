package actors

import (
	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
)

type subscriberPidHolder map[string]*actor.PID
type subscriptionHolder map[string]subscriberPidHolder

type SubscriptionHandler struct {
	middleware.LogAwareHolder
	subscriptions subscriptionHolder
}

func NewSubscriptionHandlerProps() *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &SubscriptionHandler{
			subscriptions: make(subscriptionHolder),
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (sh *SubscriptionHandler) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.TipSubscription:
		if msg.Unsubscribe {
			sh.unsubscribe(context, msg)
		} else {
			sh.subscribe(context, msg)
		}
	case *messages.CurrentStateWrapper:
		sh.notifySubscribers(context, msg)
	}
}

func (sh *SubscriptionHandler) subscribe(context actor.Context, msg *messages.TipSubscription) {
	subs, ok := sh.subscriptions[string(msg.ObjectID)]
	if !ok {
		subs = make(subscriberPidHolder)
	}
	subs[context.Sender().String()] = context.Sender()
	sh.subscriptions[string(msg.ObjectID)] = subs
}

func (sh *SubscriptionHandler) unsubscribe(context actor.Context, msg *messages.TipSubscription) {
	subs, ok := sh.subscriptions[string(msg.ObjectID)]
	if !ok {
		return
	}
	delete(subs, context.Sender().String())
}

func (sh *SubscriptionHandler) notifySubscribers(context actor.Context, msg *messages.CurrentStateWrapper) {
	subs := sh.subscriptions[string(msg.CurrentState.Signature.ObjectID)]
	for _, sub := range subs {
		context.Request(sub, msg.CurrentState)
	}
}
