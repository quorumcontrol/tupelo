package actors

import (
	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
)

// PushSyncer is the main remote-facing actor that handles
// Sending out syncs
type Validator struct {
	middleware.LogAwareHolder

	storage *actor.PID
}

func NewValidatorProps(storage *actor.PID) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &Validator{
			storage: storage,
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (v *Validator) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		v.Log.Debugw("started")
	case *messages.Store:
		v.handleStore(context, msg)
	}
}

func (v *Validator) handleStore(context actor.Context, msg *messages.Store) {
	v.Log.Debugw("validator handle store", "id", context.Self().GetId())
	// for now we're just saying all messages are valid
	// but here's where we'd decode and test, etc
	v.Log.Infow("store", "key", msg.Key)
	v.storage.Tell(msg)
}
