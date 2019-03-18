package actors

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/opentracing/opentracing-go"
	"github.com/quorumcontrol/differencedigest/ibf"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
)

type setContext struct {
	context context.Context
}

// PushSyncer is the main remote-facing actor that handles
// Sending out syncs
type PushSyncer struct {
	middleware.LogAwareHolder

	start          time.Time
	kind           string
	storageActor   *actor.PID
	remote         *actor.PID
	sendingObjects bool
	context        context.Context
}

type contextPushSyncerKey struct{}

var parentPushSyncerKey = contextPushSyncerKey{}

func (ps *PushSyncer) startInitiatorTrace() opentracing.Span {
	parent, ctx := opentracing.StartSpanFromContext(context.Background(), "push-syncer")
	ctx = context.WithValue(ctx, parentPushSyncerKey, parent)
	ps.context = ctx
	return parent
}

func (ps *PushSyncer) stopTrace() {
	val := ps.context.Value(parentPushSyncerKey)
	val.(opentracing.Span).Finish()
}

func (ps *PushSyncer) newSpan(name string) opentracing.Span {
	sp, ctx := opentracing.StartSpanFromContext(ps.context, name)
	ps.context = ctx
	return sp
}

func (ps *PushSyncer) serializedContext() (map[string]string, error) {
	serializedContext := make(map[string]string)
	sp := opentracing.SpanFromContext(ps.context)
	err := opentracing.GlobalTracer().Inject(sp.Context(), opentracing.TextMap, opentracing.TextMapCarrier(serializedContext))
	if err != nil {
		return nil, fmt.Errorf("error injecting: %v", err)
	}
	ps.Log.Debugw("serialized", "obj", serializedContext)
	return serializedContext, nil
}

func (ps *PushSyncer) setContext(ctx context.Context) {
	parent := opentracing.SpanFromContext(ctx)
	ctx = context.WithValue(ctx, parentPushSyncerKey, parent)
	ps.context = ctx
}

func (ps *PushSyncer) LogKV(key string, value interface{}) {
	sp := opentracing.SpanFromContext(ps.context)
	sp.LogKV(key, value)
}

func stopDecider(reason interface{}) actor.Directive {
	middleware.Log.Infow("actor died", "reason", reason)
	return actor.StopDirective
}

func NewPushSyncerProps(kind string, storageActor *actor.PID) *actor.Props {
	supervisor := actor.NewOneForOneStrategy(1, 10, stopDecider)

	return actor.PropsFromProducer(func() actor.Actor {
		return &PushSyncer{
			storageActor: storageActor,
			kind:         kind,
		}
	}).WithReceiverMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	).WithSupervisor(supervisor)
}

var syncerReceiveTimeout = 10 * time.Second

func (syncer *PushSyncer) Receive(context actor.Context) {
	// this makes sure the protocol continues along
	// and will terminate when nothing is happening

	switch msg := context.Message().(type) {
	case *actor.Started:
		context.SetReceiveTimeout(syncerReceiveTimeout)
	case *actor.ReceiveTimeout:
		syncer.Log.Infow("timeout")
		syncer.poison(context)
	case *messages.DoPush:
		context.SetReceiveTimeout(syncerReceiveTimeout)
		syncer.handleDoPush(context, msg)
	case *messages.ProvideStrata:
		context.SetReceiveTimeout(syncerReceiveTimeout)
		syncer.handleProvideStrata(context, msg)
	case *messages.RequestIBF:
		context.SetReceiveTimeout(syncerReceiveTimeout)
		syncer.handleRequestIBF(context, msg)
	case *messages.ProvideBloomFilter:
		context.SetReceiveTimeout(syncerReceiveTimeout)
		syncer.handleProvideBloomFilter(context, msg)
	case *messages.RequestKeys:
		context.SetReceiveTimeout(syncerReceiveTimeout)
		syncer.handleRequestKeys(context, msg)
	case *messages.Debug:
		syncer.Log.Debugf("message: %v", msg.Message)
	case *messages.SendingDone:
		syncer.sendingObjects = false
		context.Request(context.Self(), &messages.SyncDone{})
	case *messages.SyncDone:
		syncer.Log.Debugw("sync complete", "remote", syncer.remote, "length", time.Since(syncer.start))
		context.CancelReceiveTimeout()
		if !syncer.sendingObjects {
			syncer.poison(context)
		}
	case *setContext:
		syncer.setContext(msg.context)
	}
}

func (syncer *PushSyncer) poison(context actor.Context) {
	syncer.stopTrace()
	context.Self().Poison()
}

func (syncer *PushSyncer) stop(context actor.Context) {
	syncer.stopTrace()
	context.Self().Stop()
}

func (syncer *PushSyncer) handleDoPush(context actor.Context, msg *messages.DoPush) {
	sp := syncer.startInitiatorTrace()
	syncer.start = time.Now()
	syncer.Log.Debugw("sync start", "now", syncer.start)
	var remoteGossiper *actor.PID
	for remoteGossiper == nil || strings.HasPrefix(context.Self().GetId(), remoteGossiper.GetId()) {
		remoteGossiper = msg.System.GetRandomSyncer()
	}
	syncer.remote = remoteGossiper
	syncer.Log.Debugw("requesting syncer", "remote", remoteGossiper.Id)
	sp.SetTag("remote", remoteGossiper.String())

	serialized, err := syncer.serializedContext()
	if err != nil {
		syncer.Log.Errorw("error serializing context", "err", err)
	}

	resp, err := context.RequestFuture(remoteGossiper, &messages.GetSyncer{
		Kind:    syncer.kind,
		Context: serialized,
	}, 10*time.Second).Result()
	if err != nil {
		syncer.Log.Errorw("timeout waiting for remote syncer", "err", err, "remote", remoteGossiper.String())
		sp.SetTag("error", true)
		sp.SetTag("timeout", true)
		syncer.stop(context)
		return
	}

	switch remoteSyncer := resp.(type) {
	case *messages.NoSyncersAvailable:
		syncer.Log.Debugw("remote busy")
		sp.SetTag("unavailable", true)
		syncer.poison(context)
	case *messages.SyncerAvailable:
		destination := extmsgs.FromActorPid(remoteSyncer.Destination)
		syncer.Log.Debugw("requesting strata")
		strata, err := context.RequestFuture(syncer.storageActor, &messages.GetStrata{}, 2*time.Second).Result()
		if err != nil {
			syncer.Log.Errorw("timeout waiting for strata", "err", err)
			syncer.syncDone(context)
			return
		}
		syncer.Log.Debugw("providing strata", "remote", destination)
		context.Request(destination, &messages.ProvideStrata{
			Strata: strata.(*ibf.DifferenceStrata),
			DestinationHolder: messages.DestinationHolder{
				Destination: extmsgs.ToActorPid(syncer.storageActor),
			},
		})
	default:
		syncer.Log.Errorw("unknown response type", "remoteSyncer", remoteSyncer)
	}
}

func (syncer *PushSyncer) handleProvideStrata(context actor.Context, msg *messages.ProvideStrata) {
	sp := syncer.newSpan("handleProvideStrata")
	defer sp.Finish()

	syncer.Log.Debugw("handleProvideStrata")
	syncer.start = time.Now()
	syncer.remote = context.Sender()

	localStrataInt, err := context.RequestFuture(syncer.storageActor, &messages.GetStrata{}, 2*time.Second).Result()
	if err != nil {
		syncer.Log.Errorw("timeout waiting for strata", "err", err)
		syncer.syncDone(context)
		return
	}
	syncer.Log.Debugw("estimating strata")
	localStrata := localStrataInt.(*ibf.DifferenceStrata)
	count, result := localStrata.Estimate(msg.Strata)
	sp.SetTag("count", count)
	if result == nil {
		syncer.Log.Debugw("nil result")
		if count > 0 {
			wantsToSend := count * 2
			var sizeToSend int

			for _, size := range standardIBFSizes {
				if size >= wantsToSend {
					sizeToSend = size
					break
				}
			}
			if sizeToSend == 0 {
				syncer.Log.Errorf("estimate too large to send an IBF: %d", count)
				syncer.syncDone(context)
				return
			}
			sp.SetTag("IBFSize", sizeToSend)

			localIBF, err := syncer.getLocalIBF(context, sizeToSend)
			if err != nil {
				syncer.Log.Errorw("error getting local IBF", "err", err)
				syncer.syncDone(context)
				return
			}

			context.Request(context.Sender(), &messages.ProvideBloomFilter{
				Filter: localIBF,
				DestinationHolder: messages.DestinationHolder{
					Destination: extmsgs.ToActorPid(syncer.storageActor),
				},
			})
		} else {
			sp.SetTag("synced", true)
			syncer.Log.Debugw("synced", "remote", context.Sender())
			syncer.syncDone(context)
		}
	} else {
		if len(result.LeftSet) == 0 && len(result.RightSet) == 0 {
			sp.SetTag("synced", true)
			syncer.Log.Debugw("synced", "remote", context.Sender())
			syncer.syncDone(context)
		} else {
			syncer.Log.Debugw("strata", "count", count, "resultL", len(result.LeftSet), "resultR", len(result.RightSet))
			syncer.handleDiff(context, *result, extmsgs.FromActorPid(msg.Destination))
		}
	}
}

func (syncer *PushSyncer) handleRequestIBF(context actor.Context, msg *messages.RequestIBF) {
	sp := syncer.newSpan("handleRequestIBF")
	defer sp.Finish()
	syncer.Log.Debugw("handleRequestIBF")
	wantsToSend := msg.Count * 2
	var sizeToSend int

	for _, size := range standardIBFSizes {
		if size >= wantsToSend {
			sizeToSend = size
			break
		}
	}
	if sizeToSend == 0 {
		syncer.Log.Errorf("estimate too large to send an IBF: %d", msg.Count)
		syncer.syncDone(context)
		return
	}
	localIBF, err := syncer.getLocalIBF(context, sizeToSend)
	if err != nil {
		syncer.Log.Errorw("error getting local IBF", "err", err)
		syncer.syncDone(context)
		return
	}

	context.Request(context.Sender(), &messages.ProvideBloomFilter{
		Filter: localIBF,
		DestinationHolder: messages.DestinationHolder{
			Destination: extmsgs.ToActorPid(syncer.storageActor),
		},
	})
}

func (syncer *PushSyncer) handleProvideBloomFilter(context actor.Context, msg *messages.ProvideBloomFilter) {
	sp := syncer.newSpan("handleProvideBloomFilter")
	defer sp.Finish()

	localIBF, err := syncer.getLocalIBF(context, len(msg.Filter.Cells))
	if err != nil {
		syncer.Log.Errorw("error getting local IBF", "err", err)
		syncer.syncDone(context)
		return
	}
	subtracted := localIBF.Subtract(msg.Filter)
	diff, err := subtracted.Decode()
	if err != nil {
		syncer.Log.Errorw("error getting diff", "peer", context.Sender(), "diff", len(msg.Filter.Cells), "err", err)
		syncer.syncDone(context)
		return
	}
	syncer.handleDiff(context, diff, extmsgs.FromActorPid(msg.Destination))
}

func (syncer *PushSyncer) handleRequestKeys(context actor.Context, msg *messages.RequestKeys) {
	sp := syncer.newSpan("handleRequestKeys")
	defer sp.Finish()
	syncer.sendPrefixes(context, msg.Keys, context.Sender())
}

func (syncer *PushSyncer) getLocalIBF(context actor.Context, size int) (*ibf.InvertibleBloomFilter, error) {
	sp := syncer.newSpan("getLocalIBF")
	defer sp.Finish()
	localIBF, err := context.RequestFuture(syncer.storageActor, &messages.GetIBF{
		Size: size,
	}, 30*time.Second).Result()
	if err != nil {
		return nil, fmt.Errorf("error: timeout")
	}
	libf := localIBF.(*ibf.InvertibleBloomFilter)
	return libf, err
}

func (syncer *PushSyncer) handleDiff(context actor.Context, diff ibf.DecodeResults, destination *actor.PID) {
	sp := syncer.newSpan("handleDiff")
	defer sp.Finish()
	syncer.Log.Debugw("handleDiff")
	syncer.sendingObjects = true
	context.RequestWithCustomSender(context.Sender(), requestKeysFromDiff(diff.RightSet), syncer.storageActor)
	prefixes := make([]uint64, len(diff.LeftSet))
	for i, pref := range diff.LeftSet {
		prefixes[i] = uint64(pref)
	}
	syncer.sendPrefixes(context, prefixes, destination)
}

func (syncer *PushSyncer) sendPrefixes(context actor.Context, prefixes []uint64, destination *actor.PID) {
	sp := syncer.newSpan("sendPrefixes")
	defer sp.Finish()
	sender := context.SpawnPrefix(NewObjectSenderProps(syncer.storageActor), "objectSender")
	for _, pref := range prefixes {
		context.Send(sender, &messages.SendPrefix{
			Prefix:      uint64ToBytes(pref),
			Destination: destination,
		})
	}
	context.Request(sender, &messages.SendingDone{})
}

func (syncer *PushSyncer) syncDone(context actor.Context) {
	sender := context.Sender()
	syncer.Log.Debugw("sending sync complete", "remote", sender)
	if sender != nil {
		context.Request(context.Sender(), &messages.SyncDone{})
	}
	syncer.poison(context)
}

func requestKeysFromDiff(objs []ibf.ObjectId) *messages.RequestKeys {
	ints := make([]uint64, len(objs))
	for i, objID := range objs {
		ints[i] = uint64(objID)
	}
	return &messages.RequestKeys{Keys: ints}
}
