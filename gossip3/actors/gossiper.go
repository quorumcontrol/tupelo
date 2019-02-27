package actors

import (
	"fmt"
	"strings"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
)

type system interface {
	GetRandomSyncer() *actor.PID
}

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
	pusherProps      *actor.Props

	// it is expected the storageActor is actually
	// fronted with a validator
	// and will also report when its busy or not (validatorClear / validatorWorking)
	storageActor *actor.PID
}

const maxSyncers = 3

func NewGossiperProps(kind string, storage *actor.PID, system system, pusherProps *actor.Props) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &Gossiper{
			kind:             kind,
			pids:             make(map[string]*actor.PID),
			syncersAvailable: maxSyncers,
			storageActor:     storage,
			system:           system,
			pusherProps:      pusherProps,
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

type internalPusherDone struct{}

func (g *Gossiper) Receive(context actor.Context) {
	// defer func() {
	// 	if r := recover(); r != nil {
	// 		g.Log.Errorw("recover", "r", r)
	// 		panic(r)
	// 	}
	// }()
	switch msg := context.Message().(type) {
	case *actor.Restarting:
		g.Log.Infow("restarting")
	case *actor.Started:
	case *actor.Terminated:
		// this is for when the pushers stop, we can queue up another push
		if _, ok := g.pids[currentPusherKey]; ok && msg.Who.Equal(g.pids[currentPusherKey]) {
			g.Log.Debugw("terminate", "doGossip", g.validatorClear)
			delete(g.pids, currentPusherKey)
			timer := time.After(100 * time.Millisecond)
			go func(pid *actor.PID) {
				<-timer
				pid.Tell(&messages.DoOneGossip{})
			}(context.Self())

			return
		}
		if strings.HasPrefix(msg.Who.GetId(), context.Self().GetId()+"/"+remoteSyncerPrefix) {
			g.Log.Debugw("releasing a new remote syncer")
			g.syncersAvailable++
			return
		}
		panic(fmt.Sprintf("unknown actor terminated: %s", msg.Who.GetId()))

	case *messages.StartGossip:
		g.Log.Infow("start gossip")
		g.validatorClear = true
		context.Self().Tell(&messages.DoOneGossip{
			Why: "startGosip",
		})
	case *messages.DoOneGossip:
		if _, ok := g.pids[currentPusherKey]; ok {
			g.Log.Infow("ignoring because in progress")
			return
		}
		g.Log.Debugw("gossiping again")
		localsyncer, err := context.SpawnNamed(g.pusherProps, "pushSyncer")
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
			receiveSyncer := context.SpawnPrefix(g.pusherProps, remoteSyncerPrefix)
			context.Watch(receiveSyncer)
			g.syncersAvailable--
			available := &messages.SyncerAvailable{}
			available.SetDestination(messages.ToActorPid(receiveSyncer))
			context.Respond(available)
		} else {
			context.Respond(&messages.NoSyncersAvailable{})
		}
	case *messages.Subscribe:
		context.Forward(g.storageActor)
	}
}
