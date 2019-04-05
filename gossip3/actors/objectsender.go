package actors

import (
	"context"
	"fmt"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/storage"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-client/tracing"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
)

// Sender sends objects off to somewhere else
type ObjectSender struct {
	tracing.ContextHolder

	middleware.LogAwareHolder
	reader storage.Reader
	store  *actor.PID
}

func NewObjectSenderProps(ctx context.Context, store *actor.PID) *actor.Props {
	return actor.PropsFromProducer(func() actor.Actor {
		os := &ObjectSender{
			store: store,
		}
		os.SetContext(ctx)
		return os
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
		sp := o.NewSpan("objectsender-sendPrefix")
		defer sp.Finish()
		storageSpan := o.NewSpan("objectsender-GetPairsByPrefix")
		keys, err := o.reader.GetPairsByPrefix(msg.Prefix)
		storageSpan.Finish()
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
