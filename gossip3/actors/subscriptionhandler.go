package actors

import (
	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
)

type subscriptionHolder map[string][]*actor.PID

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
		sh.subscribe(context, msg)
	case *messages.CurrentStateWrapper:
		sh.notifySubscribers(context, msg)
	}
}

func (sh *SubscriptionHandler) subscribe(context actor.Context, msg *messages.TipSubscription) {
	subs := sh.subscriptions[string(msg.ObjectID)]
	sh.subscriptions[string(msg.ObjectID)] = append(subs, context.Sender())
}

func (sh *SubscriptionHandler) notifySubscribers(context actor.Context, msg *messages.CurrentStateWrapper) {
	subs := sh.subscriptions[string(msg.CurrentState.Signature.ObjectID)]
	for _, sub := range subs {
		context.Request(sub, msg.CurrentState)
	}
}
