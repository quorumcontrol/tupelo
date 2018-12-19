package actors

import (
	"encoding/binary"
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/differencedigest/ibf"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/storage"
)

var standardIBFSizes = []int{500, 2000, 100000}

type ibfMap map[int]*ibf.InvertibleBloomFilter

// PushSyncer is the main remote-facing actor that handles
// Sending out syncs
type Storage struct {
	middleware.LogAwareHolder

	id            string
	ibfs          ibfMap
	storage       *storage.MemStorage
	strata        *ibf.DifferenceStrata
	subscriptions []*actor.PID
}

func NewStorageProps() *actor.Props {
	return actor.FromProducer(NewStorage).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func NewInitializedStorageStruct() *Storage {
	s := &Storage{
		ibfs:    make(ibfMap),
		storage: storage.NewMemStorage(),
		strata:  ibf.NewDifferenceStrata(),
	}
	for _, size := range standardIBFSizes {
		s.ibfs[size] = ibf.NewInvertibleBloomFilter(size, 4)
	}
	return s
}

func NewStorage() actor.Actor {
	return NewInitializedStorageStruct()
}

func (s *Storage) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		s.id = context.Self().Id
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
	didSet, err := s.storage.SetIfNotExists(key, value)
	if err != nil {
		s.Log.Errorf("error setting: %v", err)
		return false, fmt.Errorf("error setting storage: %v", err)
	}

	if didSet {
		ibfObjectID := byteToIBFsObjectId(key[0:8])
		s.strata.Add(ibfObjectID)
		for _, filter := range s.ibfs {
			filter.Add(ibfObjectID)
		}
	} else {
		s.Log.Debugf("%s skipped adding, already exists %v", s.id, key)
	}
	return didSet, nil
}

func (s *Storage) Remove(key []byte) {
	ibfObjectID := byteToIBFsObjectId(key[0:8])
	err := s.storage.Delete(key)
	if err != nil {
		panic("storage failed")
	}
	s.strata.Remove(ibfObjectID)
	for _, filter := range s.ibfs {
		filter.Remove(ibfObjectID)
	}
	s.Log.Debugf("removed key %v", key)
}

func (s *Storage) BulkRemove(objectIDs [][]byte) {
	s.Log.Debugw("bulk remove", "objectIDs", objectIDs)
	for _, id := range objectIDs {
		s.Remove(id)
	}
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
