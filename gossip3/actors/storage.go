package actors

import (
	"encoding/binary"
	"fmt"
	"os"
	"runtime"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/hashicorp/golang-lru"
	"github.com/quorumcontrol/differencedigest/ibf"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
)

func init() {
	// SEE: https://github.com/quorumcontrol/storage
	// Using badger suggests a minimum of 128 GOMAXPROCS, but also
	// allow the user to customize
	if os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(128)
	}
}

const recentlyRemovedCacheSize = 100000

var standardIBFSizes = []int{500, 2000, 100000}

type ibfMap map[int]*ibf.InvertibleBloomFilter

// PushSyncer is the main remote-facing actor that handles
// Sending out syncs
type Storage struct {
	middleware.LogAwareHolder

	id              string
	ibfs            ibfMap
	storage         storage.Storage
	strata          *ibf.DifferenceStrata
	subscriptions   []*actor.PID
	recentlyRemoved *lru.Cache
}

func NewStorageProps(store storage.Storage) *actor.Props {
	cache, err := lru.New(recentlyRemovedCacheSize)
	if err != nil {
		panic(fmt.Errorf("error generating lru: %v", err))
	}
	return actor.FromProducer(func() actor.Actor {
		s := &Storage{
			ibfs:            make(ibfMap),
			storage:         store,
			strata:          ibf.NewDifferenceStrata(),
			recentlyRemoved: cache,
		}
		for _, size := range standardIBFSizes {
			s.ibfs[size] = ibf.NewInvertibleBloomFilter(size, 4)
		}
		return s
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (s *Storage) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Terminated:
		s.storage.Close()
	case *actor.Started:
		s.loadIBFAtStart()
	case *messages.Debug:
		s.Log.Debugw("message: %v", msg.Message)
	case *messages.Store:
		s.Log.Debugw("adding", "key", msg.Key, "value", msg.Value)
		didSet, err := s.Add(msg.Key, msg.Value)
		if err != nil {
			panic(fmt.Sprintf("error addding (k: %s): %v", msg.Key, err))
		}
		if didSet && !msg.SkipNotify {
			for _, sub := range s.subscriptions {
				context.Forward(sub)
			}
		}
	case *messages.Subscribe:
		s.subscriptions = append(s.subscriptions, msg.Subscriber)
	case *messages.Remove:
		s.Remove(msg.Key)
	case *messages.BulkRemove:
		s.BulkRemove(msg.ObjectIDs)
	case *messages.GetStrata:
		context.Respond(snapshotStrata(s.strata))
	case *messages.GetIBF:
		context.Respond(snapshotIBF(s.ibfs[msg.Size]))
	case *messages.GetThreadsafeReader:
		context.Respond(storage.Reader(s.storage))
	case *messages.GetPrefix:
		keys, err := s.storage.GetPairsByPrefix(msg.Prefix)
		if err != nil {
			s.Log.Errorw("error getting keys", "err", err)
		}
		context.Respond(keys)
	case *messages.Get:
		val, err := s.storage.Get(msg.Key)
		if err != nil {
			s.Log.Errorw("error getting key", "err", err, "key", msg.Key)
		}
		context.Respond(val)
	}
}

func snapshotIBF(existing *ibf.InvertibleBloomFilter) *ibf.InvertibleBloomFilter {
	newIbf := ibf.NewInvertibleBloomFilter(len(existing.Cells), existing.HashCount)
	copy(newIbf.Cells, existing.Cells)
	return newIbf
}

func snapshotStrata(existing *ibf.DifferenceStrata) *ibf.DifferenceStrata {
	newStrata := ibf.NewDifferenceStrata()
	for i, filt := range existing.Filters {
		newStrata.Filters[i] = snapshotIBF(filt)
	}
	return newStrata
}

func (s *Storage) Add(key, value []byte) (bool, error) {
	if s.recentlyRemoved.Contains(string(key)) {
		return false, nil
	}
	didSet, err := s.storage.SetIfNotExists(key, value)
	if err != nil {
		s.Log.Errorf("error setting: %v", err)
		return false, fmt.Errorf("error setting storage: %v", err)
	}

	if didSet {
		s.addKeyToIBFs(key)
	} else {
		s.Log.Debugf("skipped adding, already exists %v", key)
	}
	return didSet, nil
}

func (s *Storage) addKeyToIBFs(key []byte) {
	ibfObjectID := byteToIBFsObjectId(key[0:8])
	s.strata.Add(ibfObjectID)
	for _, filter := range s.ibfs {
		filter.Add(ibfObjectID)
	}
}

func (s *Storage) Remove(key []byte) {
	ibfObjectID := byteToIBFsObjectId(key[0:8])
	exists, err := s.storage.Exists(key)
	if err != nil {
		panic("storage failed")
	}
	if !exists {
		return
	}
	err = s.storage.Delete(key)
	if err != nil {
		panic("storage failed")
	}
	s.strata.Remove(ibfObjectID)
	for _, filter := range s.ibfs {
		filter.Remove(ibfObjectID)
	}
	s.recentlyRemoved.Add(string(key), nil)
	s.Log.Debugf("removed key %v", key)
}

func (s *Storage) BulkRemove(objectIDs [][]byte) {
	s.Log.Debugw("bulk remove", "objectIDs", objectIDs)
	for _, id := range objectIDs {
		s.Remove(id)
	}
}

func (s *Storage) loadIBFAtStart() {
	s.storage.ForEachKey([]byte{}, func(key []byte) error {
		s.addKeyToIBFs(key)
		return nil
	})
}

func byteToIBFsObjectId(byteID []byte) ibf.ObjectId {
	if len(byteID) != 8 {
		panic("invalid byte id sent")
	}
	return ibf.ObjectId(bytesToUint64(byteID))
}

func bytesToUint64(byteID []byte) uint64 {
	return binary.BigEndian.Uint64(byteID)
}

func uint64ToBytes(id uint64) []byte {
	a := make([]byte, 8)
	binary.BigEndian.PutUint64(a, id)
	return a
}
