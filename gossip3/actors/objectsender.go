package actors

import (
	"fmt"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/storage"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
)

// Sender sends objects off to somewhere else
type ObjectSender struct {
	middleware.LogAwareHolder
	reader storage.Reader
	store  *actor.PID
}

func NewObjectSenderProps(store *actor.PID) *actor.Props {
	return actor.PropsFromProducer(func() actor.Actor {
		return &ObjectSender{
			store: store,
		}
	}).WithReceiverMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (o *ObjectSender) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		reader, err := context.RequestFuture(o.store, &messages.GetThreadsafeReader{}, 5*time.Second).Result()
		if err != nil {
			panic(fmt.Sprintf("timeout waiting: %v", err))
		}
		o.reader = reader.(storage.Reader)
	case *actor.Restarting:
		o.Log.Errorw("restaring obj sender")
	case *messages.SendingDone:
		context.Respond(msg)
	case *messages.SendPrefix:
		keys, err := o.reader.GetPairsByPrefix(msg.Prefix)
		if err != nil {
			panic("error getting prefix")
		}
		for _, pair := range keys {
			context.Request(msg.Destination, &extmsgs.Store{
				Key:   pair.Key,
				Value: pair.Value,
			})
		}
	}
}
