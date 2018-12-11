package actors

import (
	"fmt"
	"strings"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
)

type system interface {
	GetRandomSyncer() *actor.PID
}

const remoteSyncerPrefix = "remoteSyncer"
const currentPusherKey = "currentPusher"

// Gossiper is the root gossiper
type Gossiper struct {
	middleware.LogAwareHolder
	pids             map[string]*actor.PID
	system           system
	syncersAvailable int64
}

const maxSyncers = 3

func NewGossiper() actor.Actor {
	return &Gossiper{
		pids:             make(map[string]*actor.PID),
		syncersAvailable: maxSyncers,
	}
}

var GossiperProps *actor.Props = actor.FromProducer(NewGossiper).WithMiddleware(
	middleware.LoggingMiddleware,
	plugin.Use(&middleware.LogPlugin{}),
)

func (g *Gossiper) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Terminated:
		// this is for when the pushers stop, we can queue up another push
		g.Log.Debugw("termianted", "me", context.Self().GetId(), "msg", msg)
		if msg.Who.Equal(g.pids[currentPusherKey]) {
			g.Log.Debugw("gossiping again", "me", context.Self().GetId())
			time.AfterFunc(200*time.Millisecond, func() {
				context.Self().Tell(&messages.DoOneGossip{})
			})
			return
		}
		if strings.HasPrefix(msg.Who.GetId(), context.Self().GetId()+"/"+remoteSyncerPrefix) {
			g.Log.Debugw("releasing a new remote syncer", "me", context.Self().GetId())
			g.syncersAvailable++
		}
	case *actor.Started:
		store, err := context.SpawnNamed(NewStorageProps(), "storage")
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
		g.system = msg.System
		context.Self().Tell(&messages.DoOneGossip{})
	case *messages.DoOneGossip:
		localsyncer, err := context.SpawnNamed(NewPushSyncerProps(g.pids["storage"], g.pids["validator"], true), "pushSyncer")
		if err != nil {
			panic(fmt.Sprintf("error spawning: %v", err))
		}
		g.pids[currentPusherKey] = localsyncer

		localsyncer.Tell(&messages.DoPush{
			System: g.system,
		})
	case *messages.GetStorage:
		context.Respond(g.pids["storage"])
	case *messages.GetSyncer:
		g.Log.Debugw("GetSyncer", "me", context.Self().GetId(), "remote", context.Sender().GetId())
		if g.syncersAvailable > 0 {
			receiveSyncer := context.SpawnPrefix(NewPushSyncerProps(g.pids["storage"], g.pids["validator"], false), remoteSyncerPrefix)
			context.Watch(receiveSyncer)
			g.syncersAvailable--
			context.Respond(receiveSyncer)
		} else {
			context.Respond(false)
		}
		// TODO: this is where we'd limit concurrency, etc
	case *messages.Store:
		g.pids["validator"].Tell(msg)
	case *messages.Debug:
		fmt.Printf("message: %v", msg.Message)
		actor.NewPID("test", "test")
	}
}
