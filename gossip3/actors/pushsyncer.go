package actors

import (
	"fmt"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/differencedigest/ibf"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
)

// PushSyncer is the main remote-facing actor that handles
// Sending out syncs
type PushSyncer struct {
	middleware.LogAwareHolder

	storage   *actor.PID
	validator *actor.PID
	gossiper  *actor.PID
	isLocal   bool
}

func NewPushSyncerProps(storage, validator *actor.PID, isLocal bool) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &PushSyncer{
			storage:   storage,
			validator: validator,
			isLocal:   isLocal,
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (syncer *PushSyncer) Receive(context actor.Context) {
	// this makes sure the protocol continues along
	// and will terminate when nothing is happening
	context.SetReceiveTimeout(5 * time.Second)

	switch msg := context.Message().(type) {
	case *actor.ReceiveTimeout:
		syncer.Log.Debugw("timeout", "me", context.Self().GetId())
		context.Self().Poison()
	case *actor.Started:
		syncer.handleStarted(context)
	case *messages.DoPush:
		syncer.handleDoPush(context, msg)
	case *messages.ProvideStrata:
		syncer.handleProvideStrata(context, msg)
	case *messages.RequestIBF:
		syncer.handleRequestIBF(context, msg)
	case *messages.ProvideBloomFilter:
		syncer.handleProvideBloomFilter(context, msg)
	case *messages.RequestKeys:
		syncer.handleRequestKeys(context, msg)
	case *messages.Debug:
		fmt.Printf("message: %v", msg.Message)
	case *messages.SyncDone:
		context.SetReceiveTimeout(0)
		context.Self().Poison()
	}
}

func (syncer *PushSyncer) handleStarted(context actor.Context) {
	syncer.Log.Debugw("started", "id", context.Self().Id)
}

func (syncer *PushSyncer) handleDoPush(context actor.Context, msg *messages.DoPush) {
	syncer.Log.Debugw("handleDoPush", "id", context.Self().Id)
	var remoteGossiper *actor.PID
	for remoteGossiper == nil || remoteGossiper.GetId() == context.Self().GetId() {
		remoteGossiper = msg.System.GetRandomSyncer()
	}
	syncer.Log.Debugw("requesting syncer", "me", context.Self().GetId(), "remote", remoteGossiper.GetId())

	resp, err := remoteGossiper.RequestFuture(&messages.GetSyncer{}, 1*time.Second).Result()
	if err != nil {
		panic("timeout")
	}

	switch remoteSyncer := resp.(type) {
	case bool:
		syncer.Log.Debugw("remote busy", "me", context.Self().GetId())
		context.Self().Poison()
	case *actor.PID:
		strata, err := syncer.storage.RequestFuture(&messages.GetStrata{}, 2*time.Second).Result()
		if err != nil {
			panic("timeout")
		}
		remoteSyncer.Request(&messages.ProvideStrata{
			Strata:    strata.(ibf.DifferenceStrata),
			Validator: syncer.validator,
		}, context.Self())
	}
}

func (syncer *PushSyncer) handleProvideStrata(context actor.Context, msg *messages.ProvideStrata) {
	syncer.Log.Debugw("handleProvideMessage", "id", context.Self().Id)
	//TODO: 503 when too busy
	localStrataInt, err := syncer.storage.RequestFuture(&messages.GetStrata{}, 2*time.Second).Result()
	if err != nil {
		panic("timeout")
	}
	localStrata := localStrataInt.(ibf.DifferenceStrata)
	count, result := localStrata.Estimate(&msg.Strata)
	if result != nil {
		syncer.handleDiff(context, *result, msg.Validator)
	} else {
		if count > 0 {
			context.Sender().Request(&messages.RequestIBF{
				Count:  count,
				Result: result,
			}, context.Self())
		} else {
			syncer.Log.Debugw("synced", "me", context.Self().GetId())
			syncDone(context)
		}
	}
}

func (syncer *PushSyncer) handleRequestIBF(context actor.Context, msg *messages.RequestIBF) {
	syncer.Log.Debugw("handleRequestIBF", "id", context.Self().Id)
	if msg.Count == 0 {
		//TODO: this needs to cleanup and maybe queue up another sync
		syncer.Log.Debugw("count 0", "me", context.Self().GetId())
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
		syncer.Log.Errorf("%s estimate too large to send an IBF: %d", context.Self().GetId(), msg.Count)
		syncDone(context)
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
		syncer.Log.Errorw("error getting diff from peer %s (remote size: %d): %v", "id", context.Self().GetId(), "err", err)
		syncDone(context)
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
	context.Sender().Request(requestKeysFromDiff(diff.RightSet), syncer.validator)
	prefixes := make([]uint64, len(diff.LeftSet), len(diff.LeftSet))
	for i, pref := range diff.LeftSet {
		prefixes[i] = uint64(pref)
	}
	syncer.sendPrefixes(context, prefixes, destination).Wait()
	syncDone(context)
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

func syncDone(context actor.Context) {
	context.Sender().Tell(&messages.SyncDone{})
	context.Self().Poison()
}

func requestKeysFromDiff(objs []ibf.ObjectId) *messages.RequestKeys {
	ints := make([]uint64, len(objs))
	for i, objID := range objs {
		ints[i] = uint64(objID)
	}
	return &messages.RequestKeys{Keys: ints}
}
