package actors

import (
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
)

type MemPoolValidator struct {
	middleware.LogAwareHolder

	storage *actor.PID
}

func NewMemPoolValidatorProps(storage *actor.PID) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &MemPoolValidator{
			storage: storage,
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (mpv *MemPoolValidator) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.ReceiveTimeout:
		mpv.Log.Debugw("validator clear")
		context.SetReceiveTimeout(0)
		context.Parent().Tell(&messages.ValidatorClear{})
	case *messages.Store:
		context.SetReceiveTimeout(100 * time.Millisecond)
		mpv.handleStore(context, msg)
		context.Parent().Tell(&messages.ValidatorWorking{})
	}
}

func (mpv *MemPoolValidator) handleStore(context actor.Context, msg *messages.Store) {
	mpv.Log.Debugw("validator handle store", "id", context.Self().GetId())
	// for now we're just saying all messages are valid
	// but here's where we'd decode and test ownership, etc
	mpv.Log.Infow("store", "key", msg.Key)
	mpv.storage.Tell(msg)
}
