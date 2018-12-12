package actors

import (
	"fmt"
	"strings"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
)

type system interface {
	GetRandomSyncer() *actor.PID
}

type validatorPropsCreator func(storage *actor.PID) *actor.Props

const remoteSyncerPrefix = "remoteSyncer"
const currentPusherKey = "currentPusher"

// Gossiper is the root gossiper
type Gossiper struct {
	middleware.LogAwareHolder

	kind             string
	pids             map[string]*actor.PID
	system           system
	syncersAvailable int64
	validatorClear   bool
	round            uint64
	validatorMaker   validatorPropsCreator
	storageProps     *actor.Props
}

const maxSyncers = 3

func NewGossiperProps(kind string, validatorMaker validatorPropsCreator, storageProps *actor.Props) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &Gossiper{
			kind:             kind,
			pids:             make(map[string]*actor.PID),
			syncersAvailable: maxSyncers,
			validatorMaker:   validatorMaker,
			storageProps:     storageProps,
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (g *Gossiper) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Terminated:
		// this is for when the pushers stop, we can queue up another push
		if msg.Who.Equal(g.pids[currentPusherKey]) {
			delete(g.pids, currentPusherKey)
			if g.validatorClear {
				context.Self().Tell(&messages.DoOneGossip{})
			}
			return
		}
		if strings.HasPrefix(msg.Who.GetId(), context.Self().GetId()+"/"+remoteSyncerPrefix) {
			g.Log.Debugw("releasing a new remote syncer")
			g.syncersAvailable++
			return
		}
		panic(fmt.Sprintf("unknown actor terminated: %s", msg.Who.GetId()))
	case *actor.Started:
		store, err := context.SpawnNamed(g.storageProps, "storage")
		if err != nil {
			panic(fmt.Sprintf("error spawning storage: %v", err))
		}
		g.pids["storage"] = store

		validator, err := context.SpawnNamed(g.validatorMaker(store), "validator")
		if err != nil {
			panic(fmt.Sprintf("error spawning storage: %v", err))
		}
		g.pids["validator"] = validator
	case *messages.StartGossip:
		g.validatorClear = true
		g.system = msg.System
		context.Self().Tell(&messages.DoOneGossip{})
	case *messages.DoOneGossip:
		g.Log.Debugw("gossiping again")
		localsyncer, err := context.SpawnNamed(NewPushSyncerProps(g.kind, g.pids["storage"], g.pids["validator"], true), "pushSyncer")
		if err != nil {
			panic(fmt.Sprintf("error spawning: %v", err))
		}
		g.pids[currentPusherKey] = localsyncer

		localsyncer.Tell(&messages.DoPush{
			System: g.system,
		})
	case *messages.GetSyncer:
		g.Log.Debugw("GetSyncer", "remote", context.Sender().GetId())
		if g.syncersAvailable > 0 {
			receiveSyncer := context.SpawnPrefix(NewPushSyncerProps(g.kind, g.pids["storage"], g.pids["validator"], false), remoteSyncerPrefix)
			context.Watch(receiveSyncer)
			g.syncersAvailable--
			context.Respond(receiveSyncer)
		} else {
			context.Respond(false)
		}
	case *messages.Store:
		g.pids["validator"].Tell(msg)
	case *messages.Get:
		context.Forward(g.pids["storage"])
	case *messages.ValidatorClear:
		g.validatorClear = true
		_, ok := g.pids[currentPusherKey]
		if !ok {
			context.Self().Tell(&messages.DoOneGossip{})
		}
	case *messages.ValidatorWorking:
		g.validatorClear = false
	case *messages.Debug:
		actor.NewPID("test", "test")
	}
}