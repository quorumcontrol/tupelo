package actors

import (
	"fmt"
	"strings"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/ethereum/go-ethereum/log"
	"github.com/quorumcontrol/differencedigest/ibf"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
)

// PushSyncer is the main remote-facing actor that handles
// Sending out syncs
type PushSyncer struct {
	middleware.LogAwareHolder

	kind      string
	storage   *actor.PID
	validator *actor.PID
	gossiper  *actor.PID
	isLocal   bool
}

func NewPushSyncerProps(kind string, storage, validator *actor.PID, isLocal bool) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &PushSyncer{
			storage:   storage,
			validator: validator,
			isLocal:   isLocal,
			kind:      kind,
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (syncer *PushSyncer) Receive(context actor.Context) {
	// this makes sure the protocol continues along
	// and will terminate when nothing is happening

	switch msg := context.Message().(type) {
	case *actor.ReceiveTimeout:
		syncer.Log.Debugw("timeout")
		context.Self().Poison()
	case *messages.DoPush:
		context.SetReceiveTimeout(2 * time.Second)
		syncer.handleDoPush(context, msg)
	case *messages.ProvideStrata:
		context.SetReceiveTimeout(2 * time.Second)
		syncer.handleProvideStrata(context, msg)
	case *messages.RequestIBF:
		context.SetReceiveTimeout(2 * time.Second)
		syncer.handleRequestIBF(context, msg)
	case *messages.ProvideBloomFilter:
		context.SetReceiveTimeout(2 * time.Second)
		syncer.handleProvideBloomFilter(context, msg)
	case *messages.RequestKeys:
		context.SetReceiveTimeout(2 * time.Second)
		syncer.handleRequestKeys(context, msg)
	case *messages.Debug:
		syncer.Log.Debugf("message: %v", msg.Message)
	case *messages.SyncDone:
		context.SetReceiveTimeout(0)

		syncer.Log.Debugw("received sync done", "remote", context.Sender().GetId())
		context.SetReceiveTimeout(0)
		context.Self().Poison()
	}
}

func (syncer *PushSyncer) handleDoPush(context actor.Context, msg *messages.DoPush) {
	syncer.Log.Debugw("handleDoPush")
	var remoteGossiper *actor.PID
	for remoteGossiper == nil || strings.HasPrefix(context.Self().GetId(), remoteGossiper.GetId()) {
		remoteGossiper = msg.System.GetRandomSyncer()
	}
	syncer.Log.Debugw("requesting syncer", "remote", remoteGossiper.GetId())

	resp, err := remoteGossiper.RequestFuture(&messages.GetSyncer{
		Kind: syncer.kind,
	}, 1*time.Second).Result()
	if err != nil {
		panic("timeout")
	}

	switch remoteSyncer := resp.(type) {
	case bool:
		syncer.Log.Debugw("remote busy")
		context.Self().Poison()
	case *actor.PID:
		syncer.Log.Debugw("requesting strata")
		strata, err := syncer.storage.RequestFuture(&messages.GetStrata{}, 2*time.Second).Result()
		if err != nil {
			panic("timeout")
		}
		syncer.Log.Debugw("providing strata", "remote", remoteSyncer.GetId())
		remoteSyncer.Request(&messages.ProvideStrata{
			Strata:    strata.(ibf.DifferenceStrata),
			Validator: syncer.validator,
		}, context.Self())
	default:
		panic("unknown type")
	}
}

func (syncer *PushSyncer) handleProvideStrata(context actor.Context, msg *messages.ProvideStrata) {
	syncer.Log.Debugw("handleProvideStrata")
	//TODO: 503 when too busy
	localStrataInt, err := syncer.storage.RequestFuture(&messages.GetStrata{}, 2*time.Second).Result()
	if err != nil {
		panic("timeout")
	}
	syncer.Log.Debugw("estimating strata")
	localStrata := localStrataInt.(ibf.DifferenceStrata)
	count, result := localStrata.Estimate(&msg.Strata)
	if result == nil {
		syncer.Log.Debugw("nil result")
		if count > 0 {
			log.Debug("requesting IBF", "remote", context.Sender().GetId())
			context.Sender().Request(&messages.RequestIBF{
				Count:  count,
				Result: result,
			}, context.Self())
		} else {
			syncer.Log.Debugw("synced")
			syncer.syncDone(context)
		}
	} else {
		syncer.handleDiff(context, *result, msg.Validator)
	}
}

func (syncer *PushSyncer) handleRequestIBF(context actor.Context, msg *messages.RequestIBF) {
	syncer.Log.Debugw("handleRequestIBF")
	if msg.Count == 0 {
		//TODO: this needs to cleanup and maybe queue up another sync
		syncer.Log.Debugw("count 0")
		return
	}
	if msg.Result != nil {
		//TODO: just send a wants and then start sending the messages
	}
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
		Filter:    *localIBF,
		Validator: syncer.validator,
	}, context.Self())
}

func (syncer *PushSyncer) handleProvideBloomFilter(context actor.Context, msg *messages.ProvideBloomFilter) {
	localIBF, err := syncer.getLocalIBF(len(msg.Filter.Cells))
	if err != nil {
		panic(fmt.Sprintf("error getting local IBF: %v", err))
	}
	subtracted := localIBF.Subtract(&msg.Filter)
	diff, err := subtracted.Decode()
	if err != nil {
		syncer.Log.Errorw("error getting diff from peer %s (remote size: %d): %v", "err", err)
		syncer.syncDone(context)
	}
	syncer.handleDiff(context, diff, msg.Validator)
}

func (syncer *PushSyncer) handleRequestKeys(context actor.Context, msg *messages.RequestKeys) {
	syncer.sendPrefixes(context, msg.Keys, context.Sender())
}

func (syncer *PushSyncer) getLocalIBF(size int) (*ibf.InvertibleBloomFilter, error) {
	localIBF, err := syncer.storage.RequestFuture(&messages.GetIBF{
		Size: size,
	}, 30*time.Second).Result()
	if err != nil {
		return nil, fmt.Errorf("error: timeout")
	}
	ibf := localIBF.(ibf.InvertibleBloomFilter)
	return &ibf, err
}

func (syncer *PushSyncer) handleDiff(context actor.Context, diff ibf.DecodeResults, destination *actor.PID) {
	syncer.Log.Debugw("handleDiff")
	context.Sender().Request(requestKeysFromDiff(diff.RightSet), syncer.validator)
	prefixes := make([]uint64, len(diff.LeftSet), len(diff.LeftSet))
	for i, pref := range diff.LeftSet {
		prefixes[i] = uint64(pref)
	}
	syncer.sendPrefixes(context, prefixes, destination).Wait()
	syncer.syncDone(context)
}

func (syncer *PushSyncer) sendPrefixes(context actor.Context, prefixes []uint64, destination *actor.PID) *actor.Future {
	sender := context.SpawnPrefix(NewObjectSenderProps(syncer.storage), "objectSender")
	for _, pref := range prefixes {
		sender.Tell(&messages.SendPrefix{
			Prefix:      uint64ToBytes(pref),
			Destination: destination,
		})
	}

	return sender.PoisonFuture()
}

func (syncer *PushSyncer) syncDone(context actor.Context) {
	sender := context.Sender()
	syncer.Log.Debugw("sending sync done", "remote", sender)
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
