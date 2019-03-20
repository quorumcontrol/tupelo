package actors

import (
	"fmt"
	"strings"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/mailbox"
	"github.com/AsynkronIT/protoactor-go/plugin"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-client/tracing"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
)

type system interface {
	GetRandomSyncer() *actor.PID
}

const remoteSyncerPrefix = "remoteSyncer"
const currentPusherKey = "currentPusher"

// Gossiper is the root gossiper
type Gossiper struct {
	middleware.LogAwareHolder
	tracing.ContextHolder

	kind             string
	pids             map[string]*actor.PID
	system           system
	syncersAvailable int64
	validatorClear   bool
	pusherProps      *actor.Props

	// it is expected the storageActor is actually
	// fronted with a validator
	// and will also report when its busy or not (validatorClear / validatorWorking)
	storageActor *actor.PID
}

const maxSyncers = 3

func NewGossiperProps(kind string, storage *actor.PID, system system, pusherProps *actor.Props) *actor.Props {
	supervisor := actor.NewOneForOneStrategy(1, 10, stopDecider)

	return actor.PropsFromProducer(func() actor.Actor {
		return &Gossiper{
			kind:             kind,
			pids:             make(map[string]*actor.PID),
			syncersAvailable: maxSyncers,
			storageActor:     storage,
			system:           system,
			pusherProps:      pusherProps,
		}
	}).WithReceiverMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	).WithSupervisor(
		supervisor,
	).WithDispatcher(
		mailbox.NewSynchronizedDispatcher(300),
	)
}

func (g *Gossiper) Receive(actorContext actor.Context) {
	// defer func() {
	// 	if r := recover(); r != nil {
	// 		g.Log.Errorw("recover", "r", r)
	// 		panic(r)
	// 	}
	// }()
	switch msg := actorContext.Message().(type) {
	case *actor.Started:
		g.StartTrace("gossiper")
	case *actor.Stopping:
		g.Log.Debugw("stopping")
		g.StopTrace()
	case *actor.Restarting:
		g.Log.Infow("restarting")
	case *actor.Terminated:
		sp := g.NewSpan("terminated")
		defer sp.Finish()
		sp.SetTag("who", msg.Who.Id)
		// this is for when the pushers stop, we can queue up another push
		if _, ok := g.pids[currentPusherKey]; ok && msg.Who.Equal(g.pids[currentPusherKey]) {
			g.Log.Debugw("terminate", "doGossip", g.validatorClear)
			delete(g.pids, currentPusherKey)
			timer := time.After(100 * time.Millisecond)
			go func(pid *actor.PID) {
				<-timer
				//fmt.Printf("%s Sending DoOneGossip\n", pid.String())
				actorContext.Send(pid, &messages.DoOneGossip{})
			}(actorContext.Self())

			return
		}
		if strings.HasPrefix(msg.Who.GetId(), actorContext.Self().GetId()+"/"+remoteSyncerPrefix) {
			g.Log.Debugw("releasing a new remote syncer")
			g.syncersAvailable++
			return
		}
		g.Log.Errorw("unknown actor terminated", "who", msg.Who.GetId(), "pids", g.pids)

		panic(fmt.Sprintf("unknown actor terminated: %s", msg.Who.GetId()))

	case *messages.StartGossip:
		sp := g.NewSpan("start-gossip")
		defer sp.Finish()
		g.Log.Debugw("start gossip")
		g.validatorClear = true
		actorContext.Send(actorContext.Self(), &messages.DoOneGossip{
			Why: "startGosip",
		})
	case *messages.DoOneGossip:
		//fmt.Printf("%s Received DoOneGossip\n", context.Self().String())
		sp := g.NewSpan("doOneGossip")
		defer sp.Finish()

		if _, ok := g.pids[currentPusherKey]; ok {
			sp.SetTag("ignoring", true)
			g.Log.Debugw("ignoring because in progress")
			return
		}
		g.Log.Debugw("gossiping again")
		localsyncer, err := actorContext.SpawnNamed(g.pusherProps, "pushSyncer")
		if err != nil {
			panic(fmt.Sprintf("error spawning: %v", err))
		}
		g.pids[currentPusherKey] = localsyncer

		actorContext.Send(localsyncer, &messages.DoPush{
			System: g.system,
		})
	case *messages.GetSyncer:
		sp := g.NewSpan("getSyncer")
		defer sp.Finish()
		g.Log.Debugw("GetSyncer", "remote", actorContext.Sender().GetId())

		// syncerCtx := context.Background()
		// remoteSpan, err := tracing.SpanContextFromSerialized(msg.Context, "pushSyncer-receiver")

		// if err != nil {
		// 	g.Log.Warnw("error decoding remote context", "err", err, "msg", msg, "from", actorContext.Sender().String())
		// 	remoteSpan = opentracing.StartSpan("pushSyncer-receiver-no-parent")
		// }

		// remoteSpan.SetTag("actor", actorContext.Self().String())
		// sp.SetTag("actor", actorContext.Self().String())
		// remoteSpan.SetTag("syncersAvailable", g.syncersAvailable)
		// sp.SetTag("syncersAvailable", g.syncersAvailable)
		// syncerCtx = opentracing.ContextWithSpan(syncerCtx, remoteSpan)

		if g.syncersAvailable > 0 {
			sp.SetTag("available", true)
			receiveSyncer := actorContext.SpawnPrefix(g.pusherProps, remoteSyncerPrefix)
			actorContext.Send(receiveSyncer, &setContext{context: msg.GetContext()})
			g.syncersAvailable--
			available := &messages.SyncerAvailable{}
			available.SetDestination(extmsgs.ToActorPid(receiveSyncer))
			actorContext.Respond(available)
		} else {
			sp.SetTag("unavailable", true)
			actorContext.Respond(&messages.NoSyncersAvailable{})
		}
	case *messages.Subscribe:
		sp := g.NewSpan("subscribe")
		defer sp.Finish()
		actorContext.Forward(g.storageActor)
	}
}
