package actors

import (
	"fmt"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
)

type system interface {
	GetRandomSyncer() *actor.PID
}

// Gossiper is the root gossiper
type Gossiper struct {
	pids   map[string]*actor.PID
	system system
}

func NewGossiper() actor.Actor {
	return &Gossiper{
		pids: make(map[string]*actor.PID),
	}
}

var GossiperProps *actor.Props = actor.FromProducer(NewGossiper).WithMiddleware(middleware.LoggingMiddleware)

func (g *Gossiper) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		store, err := context.SpawnNamed(StorageProps, "storage")
		if err != nil {
			panic(fmt.Sprintf("error spawning storage: %v", err))
		}
		g.pids["storage"] = store

		validator, err := context.SpawnNamed(NewValidatorProps(store), "validator")
		if err != nil {
			panic(fmt.Sprintf("error spawning storage: %v", err))
		}
		g.pids["validator"] = validator
	case *messages.StartGossip:
		pid, err := context.SpawnNamed(NewPushSyncerProps(g.pids["storage"], g.pids["validator"]), "pushSyncer")
		if err != nil {
			panic(fmt.Sprintf("error spawning: %v", err))
		}

		var remoteGossiper *actor.PID
		for remoteGossiper == nil || remoteGossiper.GetId() == context.Self().GetId() {
			remoteGossiper = msg.System.GetRandomSyncer()
		}
		syncer, err := remoteGossiper.RequestFuture(&messages.GetSyncer{}, 30*time.Second).Result()
		if err != nil {
			panic("timeout")
		}
		pid.Tell(&messages.DoPush{
			RemoteSyncer: syncer.(*actor.PID),
		})
	case *messages.GetStorage:
		context.Respond(g.pids["storage"])
	case *messages.GetSyncer:
		// TODO: this is where we'd limit concurrency, etc
		context.Respond(context.SpawnPrefix(NewPushSyncerProps(g.pids["storage"], g.pids["validator"]), "syncer"))
	case *messages.Store:
		g.pids["validator"].Tell(msg)
	case *messages.Debug:
		fmt.Printf("message: %v", msg.Message)
	}
}
