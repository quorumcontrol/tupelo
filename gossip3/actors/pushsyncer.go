package actors

import (
	"fmt"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/differencedigest/ibf"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
)

// PushSyncer is the main remote-facing actor that handles
// Sending out syncs
type PushSyncer struct {
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
	}).WithMiddleware(middleware.LoggingMiddleware)
}

func (syncer *PushSyncer) Receive(context actor.Context) {
	// this makes sure the protocol continues along
	// and will terminate when nothing is happening
	context.SetReceiveTimeout(1 * time.Second)

	switch msg := context.Message().(type) {
	case *actor.ReceiveTimeout:
		log.Infow("timeout", "me", context.Self().GetId())
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
	log.Infow("started", "id", context.Self().Id)
}

func (syncer *PushSyncer) handleDoPush(context actor.Context, msg *messages.DoPush) {
	log.Infow("handleDoPush", "id", context.Self().Id)
	strata, err := syncer.storage.RequestFuture(&messages.GetStrata{}, 30*time.Second).Result()
	if err != nil {
		panic("timeout")
	}
	msg.RemoteSyncer.Request(&messages.ProvideStrata{
		Strata:    strata.(ibf.DifferenceStrata),
		Validator: syncer.validator,
	}, context.Self())
}

func (syncer *PushSyncer) handleProvideStrata(context actor.Context, msg *messages.ProvideStrata) {
	log.Infow("handleProvideMessage", "id", context.Self().Id)
	//TODO: 503 when too busy
	localStrataInt, err := syncer.storage.RequestFuture(&messages.GetStrata{}, 30*time.Second).Result()
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
			log.Infow("synced", "me", context.Self().GetId())
			context.Sender().Tell(&messages.SyncDone{})
		}
	}
}

func (syncer *PushSyncer) handleRequestIBF(context actor.Context, msg *messages.RequestIBF) {
	log.Infow("handleRequestIBF", "id", context.Self().Id)
	if msg.Count == 0 {
		//TODO: this needs to cleanup and maybe queue up another sync
		log.Infow("count 0", "me", context.Self().GetId())
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
		log.Errorf("%s estimate too large to send an IBF: %d", context.Self().GetId(), msg.Count)
		context.Sender().Tell(&messages.SyncDone{})
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
		log.Errorw("error getting diff from peer %s (remote size: %d): %v", "id", context.Self().GetId(), "err", err)
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
	context.Sender().Tell(&messages.SyncDone{})
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

func notifyWhenFinished(destination *actor.PID, msg interface{}) *actor.PID {
	return actor.Spawn(actor.FromFunc(func(ctx actor.Context) {
		switch msg := ctx.Message().(type) {
		case *actor.Terminated:
			destination.Tell(msg)
			ctx.Self().Stop()
		}
	}))
}

func requestKeysFromDiff(objs []ibf.ObjectId) *messages.RequestKeys {
	ints := make([]uint64, len(objs))
	for i, objID := range objs {
		ints[i] = uint64(objID)
	}
	return &messages.RequestKeys{Keys: ints}
}
