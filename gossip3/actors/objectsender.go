package actors

import (
	"fmt"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
)

// Sender sends objects off to somewhere else
type ObjectSender struct {
	store *actor.PID
}

func NewObjectSenderProps(store *actor.PID) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &ObjectSender{
			store: store,
		}
	}).WithMiddleware(middleware.LoggingMiddleware)
}

func (o *ObjectSender) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.SendPrefix:
		keys, err := o.store.RequestFuture(&messages.GetPrefix{Prefix: msg.Prefix}, 30*time.Millisecond).Result()
		if err != nil {
			panic(fmt.Sprintf("timeout waiting: %v", err))
		}
		log.Infow("sending", "me", context.Self().GetId(), "prefix", msg.Prefix, "length", len(keys.([]storage.KeyValuePair)))
		for _, pair := range keys.([]storage.KeyValuePair) {
			msg.Destination.Tell(&messages.Store{
				Key:   pair.Key,
				Value: pair.Value,
			})
		}
	}
}
