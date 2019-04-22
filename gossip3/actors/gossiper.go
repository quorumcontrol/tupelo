package actors

import (
	"fmt"
	"strings"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/mailbox"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/opentracing/opentracing-go"
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

	kind              string
	pids              map[string]*actor.PID
	system            system
	syncersAvailable  int64
	validatorClear    bool
	pusherProps       *actor.Props
	fullExchangeActor *actor.PID

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
		fullExchangeActor, err := actorContext.SpawnNamed(NewFullExchangeProps(g.storageActor), "fullExchange")
		if err != nil {
			panic(fmt.Sprintf("error spawning: %v", err))
		}
		g.fullExchangeActor = fullExchangeActor
	case *actor.Restarting:
		g.Log.Infow("restarting")
	case *actor.Terminated:
		sp := g.StartTrace("pushSyncerTerminated")
		defer g.StopTrace()
		sp.SetTag("who", msg.Who.String())
		sp.SetTag("syncersAvailable", g.syncersAvailable)

		// this is for when the pushers stop, we can queue up another push
		if _, ok := g.pids[currentPusherKey]; ok && msg.Who.Equal(g.pids[currentPusherKey]) {
			sp.SetTag("initiator", true)
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
			sp.SetTag("remoteSyncer", true)
			g.Log.Debugw("releasing a new remote syncer")
			g.syncersAvailable++
			return
		}
		sp.SetTag("error", true)
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
			sp.SetTag("error", true)
			panic(fmt.Sprintf("error spawning: %v", err))
		}
		g.pids[currentPusherKey] = localsyncer

		actorContext.Send(localsyncer, &messages.DoPush{
			System: g.system,
		})
	case *messages.RequestFullExchange:
		actorContext.Request(g.fullExchangeActor, msg)
	case *messages.ReceiveFullExchange:
		actorContext.Forward(g.fullExchangeActor)
	case *messages.GetSyncer:
		sp := g.StartTrace("getSyncer")
		defer g.StopTrace()
		g.Log.Debugw("GetSyncer", "remote", actorContext.Sender().GetId())
		sp.SetTag("remote", actorContext.Sender().String())
		sp.SetTag("syncersAvailable", g.syncersAvailable)

		if g.syncersAvailable > 0 {
			receiveSyncer := actorContext.SpawnPrefix(g.pusherProps, remoteSyncerPrefix)
			// we are using StartSpanFromContext here because ocastionally we
			// do not get the right context from the pushsyncer side.
			// This will handle the case in either good (we have a context)
			// or bad (there is no context). The span is stopped by the pushsyncer
			_, msgCtx := opentracing.StartSpanFromContext(msg.GetContext(), "receiverSyncer")
			actorContext.Send(receiveSyncer, &setContext{context: msgCtx})

			g.syncersAvailable--
			available := &messages.SyncerAvailable{}
			available.SetDestination(extmsgs.ToActorPid(receiveSyncer))
			actorContext.Respond(available)
		} else {
			sp.SetTag("unavailable", true)
			actorContext.Respond(&messages.NoSyncersAvailable{})
		}
	case *messages.Subscribe:
		actorContext.Forward(g.storageActor)
	}
}
