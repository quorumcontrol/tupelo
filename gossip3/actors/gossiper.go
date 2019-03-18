package actors

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/opentracing/opentracing-go"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
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
	case *actor.Restarting:
		g.Log.Infow("restarting")
	case *actor.Terminated:

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
		g.Log.Debugw("start gossip")
		g.validatorClear = true
		actorContext.Send(actorContext.Self(), &messages.DoOneGossip{
			Why: "startGosip",
		})
	case *messages.DoOneGossip:
		//fmt.Printf("%s Received DoOneGossip\n", context.Self().String())
		if _, ok := g.pids[currentPusherKey]; ok {
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
		g.Log.Debugw("GetSyncer", "remote", actorContext.Sender().GetId())
		ctx := context.Background()
		spanContext, err := opentracing.GlobalTracer().Extract(opentracing.TextMap, opentracing.TextMapCarrier(msg.Context))
		var sp opentracing.Span
		if err == nil {
			sp = opentracing.StartSpan("pushSyncer-receiver", opentracing.ChildOf(spanContext))
		} else {
			g.Log.Warnw("error decoding remote context", "err", err, "msg", msg, "from", actorContext.Sender().String())
			sp = opentracing.StartSpan("pushSyncer-receiver-no-parent")
		}
		sp.SetTag("actor", actorContext.Self().String())
		sp.SetTag("syncersAvailable", g.syncersAvailable)
		ctx = opentracing.ContextWithSpan(ctx, sp)

		if g.syncersAvailable > 0 {
			receiveSyncer := actorContext.SpawnPrefix(g.pusherProps, remoteSyncerPrefix)
			actorContext.Send(receiveSyncer, &setContext{context: ctx})
			g.syncersAvailable--
			available := &messages.SyncerAvailable{}
			available.SetDestination(extmsgs.ToActorPid(receiveSyncer))
			actorContext.Respond(available)
		} else {
			actorContext.Respond(&messages.NoSyncersAvailable{})
		}
	case *messages.Subscribe:
		actorContext.Forward(g.storageActor)
	}
}
