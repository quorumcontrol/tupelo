package actors

import (
	"fmt"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
)

// Sender sends objects off to somewhere else
type ObjectSender struct {
	middleware.LogAwareHolder

	store *actor.PID
}

func NewObjectSenderProps(store *actor.PID) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &ObjectSender{
			store: store,
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (o *ObjectSender) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.SendPrefix:
		keys, err := o.store.RequestFuture(&messages.GetPrefix{Prefix: msg.Prefix}, 500*time.Millisecond).Result()
		if err != nil {
			panic(fmt.Sprintf("timeout waiting: %v", err))
		}
		o.Log.Debugw("sending", "destination", msg.Destination.GetId(), "prefix", msg.Prefix, "length", len(keys.([]storage.KeyValuePair)))
		for _, pair := range keys.([]storage.KeyValuePair) {
			msg.Destination.Tell(&messages.Store{
				Key:   pair.Key,
				Value: pair.Value,
			})
		}
	}
}
