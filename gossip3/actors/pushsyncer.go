package actors

import (
	"fmt"
	"strings"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/differencedigest/ibf"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
)

// PushSyncer is the main remote-facing actor that handles
// Sending out syncs
type PushSyncer struct {
	middleware.LogAwareHolder

	start          time.Time
	kind           string
	storageActor   *actor.PID
	gossiper       *actor.PID
	remote         *actor.PID
	sendingObjects bool
}

func stopDecider(reason interface{}) actor.Directive {
	middleware.Log.Infow("actor died", "reason", reason)
	return actor.StopDirective
}

func NewPushSyncerProps(kind string, storageActor *actor.PID) *actor.Props {
	supervisor := actor.NewOneForOneStrategy(1, 10, stopDecider)

	return actor.FromProducer(func() actor.Actor {
		return &PushSyncer{
			storageActor: storageActor,
			kind:         kind,
		}
	}).WithMiddleware(
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
		context.Self().Poison()
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
		syncer.Log.Infow("sync complete", "remote", syncer.remote, "length", time.Now().Sub(syncer.start))
		context.SetReceiveTimeout(0)
		if !syncer.sendingObjects {
			context.Self().Poison()
		}
	}
}

func (syncer *PushSyncer) handleDoPush(context actor.Context, msg *messages.DoPush) {
	syncer.start = time.Now()
	syncer.Log.Debugw("sync start", "now", syncer.start)
	var remoteGossiper *actor.PID
	for remoteGossiper == nil || strings.HasPrefix(context.Self().GetId(), remoteGossiper.GetId()) {
		remoteGossiper = msg.System.GetRandomSyncer()
	}
	syncer.remote = remoteGossiper
	syncer.Log.Debugw("requesting syncer", "remote", remoteGossiper.Id)

	resp, err := remoteGossiper.RequestFuture(&messages.GetSyncer{
		Kind: syncer.kind,
	}, 30*time.Second).Result()
	if err != nil {
		syncer.Log.Errorw("timeout waiting for remote syncer", "err", err)
		panic("timeout waiting for remote syncer")
	}

	switch remoteSyncer := resp.(type) {
	case *messages.NoSyncersAvailable:
		syncer.Log.Debugw("remote busy")
		context.Self().Poison()
	case *messages.SyncerAvailable:
		destination := messages.FromActorPid(remoteSyncer.Destination)
		syncer.Log.Debugw("requesting strata")
		strata, err := syncer.storageActor.RequestFuture(&messages.GetStrata{}, 2*time.Second).Result()
		if err != nil {
			panic("timeout")
		}
		syncer.Log.Debugw("providing strata", "remote", destination)
		destination.Request(&messages.ProvideStrata{
			Strata:            strata.(*ibf.DifferenceStrata),
			DestinationHolder: messages.DestinationHolder{messages.ToActorPid(syncer.storageActor)},
		}, context.Self())
	default:
		panic("unknown type")
	}
}

func (syncer *PushSyncer) handleProvideStrata(context actor.Context, msg *messages.ProvideStrata) {
	syncer.Log.Debugw("handleProvideStrata")
	syncer.start = time.Now()
	syncer.remote = context.Sender()

	localStrataInt, err := syncer.storageActor.RequestFuture(&messages.GetStrata{}, 2*time.Second).Result()
	if err != nil {
		panic("timeout")
	}
	syncer.Log.Debugw("estimating strata")
	localStrata := localStrataInt.(*ibf.DifferenceStrata)
	count, result := localStrata.Estimate(msg.Strata)
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
			localIBF, err := syncer.getLocalIBF(sizeToSend)
			if err != nil {
				panic("timeout")
			}

			context.Request(context.Sender(), &messages.ProvideBloomFilter{
				Filter:            localIBF,
				DestinationHolder: messages.DestinationHolder{messages.ToActorPid(syncer.storageActor)},
			})
		} else {
			syncer.Log.Debugw("synced", "remote", context.Sender())
			syncer.syncDone(context)
		}
	} else {
		if len(result.LeftSet) == 0 && len(result.RightSet) == 0 {
			syncer.Log.Debugw("synced", "remote", context.Sender())
			syncer.syncDone(context)
		} else {
			syncer.Log.Debugw("strata", "count", count, "resultL", len(result.LeftSet), "resultR", len(result.RightSet))
			syncer.handleDiff(context, *result, messages.FromActorPid(msg.Destination))
		}

	}
}

func (syncer *PushSyncer) handleRequestIBF(context actor.Context, msg *messages.RequestIBF) {
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
	localIBF, err := syncer.getLocalIBF(sizeToSend)
	if err != nil {
		panic("timeout")
	}

	context.Sender().Request(&messages.ProvideBloomFilter{
		Filter:            localIBF,
		DestinationHolder: messages.DestinationHolder{messages.ToActorPid(syncer.storageActor)},
	}, context.Self())
}

func (syncer *PushSyncer) handleProvideBloomFilter(context actor.Context, msg *messages.ProvideBloomFilter) {
	localIBF, err := syncer.getLocalIBF(len(msg.Filter.Cells))
	if err != nil {
		panic(fmt.Sprintf("error getting local IBF: %v", err))
	}
	subtracted := localIBF.Subtract(msg.Filter)
	diff, err := subtracted.Decode()
	if err != nil {
		syncer.Log.Errorw("error getting diff", "peer", context.Sender(), "diff", len(msg.Filter.Cells), "err", err)
		syncer.syncDone(context)
		return
	}
	syncer.handleDiff(context, diff, messages.FromActorPid(msg.Destination))
}

func (syncer *PushSyncer) handleRequestKeys(context actor.Context, msg *messages.RequestKeys) {
	syncer.sendPrefixes(context, msg.Keys, context.Sender())
}

func (syncer *PushSyncer) getLocalIBF(size int) (*ibf.InvertibleBloomFilter, error) {
	localIBF, err := syncer.storageActor.RequestFuture(&messages.GetIBF{
		Size: size,
	}, 30*time.Second).Result()
	if err != nil {
		return nil, fmt.Errorf("error: timeout")
	}
	ibf := localIBF.(*ibf.InvertibleBloomFilter)
	return ibf, err
}

func (syncer *PushSyncer) handleDiff(context actor.Context, diff ibf.DecodeResults, destination *actor.PID) {
	syncer.Log.Debugw("handleDiff")
	syncer.sendingObjects = true
	context.Sender().Request(requestKeysFromDiff(diff.RightSet), syncer.storageActor)
	prefixes := make([]uint64, len(diff.LeftSet), len(diff.LeftSet))
	for i, pref := range diff.LeftSet {
		prefixes[i] = uint64(pref)
	}
	syncer.sendPrefixes(context, prefixes, destination)
}

func (syncer *PushSyncer) sendPrefixes(context actor.Context, prefixes []uint64, destination *actor.PID) {
	sender := context.SpawnPrefix(NewObjectSenderProps(syncer.storageActor), "objectSender")
	for _, pref := range prefixes {
		sender.Tell(&messages.SendPrefix{
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
		context.Sender().Request(&messages.SyncDone{}, context.Self())
	}
	context.Self().Poison()
}

func requestKeysFromDiff(objs []ibf.ObjectId) *messages.RequestKeys {
	ints := make([]uint64, len(objs))
	for i, objID := range objs {
		ints[i] = uint64(objID)
	}
	return &messages.RequestKeys{Keys: ints}
}
