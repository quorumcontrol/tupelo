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

type ibfMap map[int]*ibf.InvertibleBloomFilter

var standardIBFSizes = []int{500, 2000, 100000}

// PushSyncer is the main remote-facing actor that handles
// Sending out syncs
type Storage struct {
	middleware.LogAwareHolder

	id      string
	ibfs    ibfMap
	storage *storage.MemStorage
	strata  *ibf.DifferenceStrata
}

func NewStorageProps() *actor.Props {
	return actor.FromProducer(NewStorage).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func NewStorage() actor.Actor {
	s := &Storage{
		ibfs:    make(ibfMap),
		storage: storage.NewMemStorage(),
		strata:  ibf.NewDifferenceStrata(),
	}
	for _, size := range standardIBFSizes {
		s.ibfs[size] = ibf.NewInvertibleBloomFilter(size, 4)
		// node.IBFs[size].TurnOnDebug()
	}
	return s
}

func (s *Storage) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		s.id = context.Self().Id
		s.Log.Debugw("started", "me", context.Self().Id)
	case *messages.Debug:
		s.Log.Debugw("message: %v", msg.Message)
	case *messages.Store:
		s.Log.Debugw("adding", "me", context.Self().GetId(), "key", msg.Key, "value", msg.Value)
		s.Add(msg.Key, msg.Value)
	case *messages.Remove:
		s.Remove(msg.Key)
	case *messages.GetStrata:
		context.Respond(*s.strata)
	case *messages.GetIBF:
		context.Respond(*s.ibfs[msg.Size])
	case *messages.GetPrefix:
		keys, err := s.storage.GetPairsByPrefix(msg.Prefix)
		if err != nil {
			s.Log.Errorw("error getting keys", "me", context.Self().GetId(), "err", err)
		}
		context.Respond(keys)
	case *messages.Get:
		val, err := s.storage.Get(msg.Key)
		s.Log.Debugw("get", "id", s.id, "key", msg.Key, "val", val)
		if err != nil {
			s.Log.Errorw("error getting key", "me", context.Self().GetId(), "err", err, "key", msg.Key)
		}
		context.Respond(val)
	}
}

func (s *Storage) Add(key, value []byte) (bool, error) {
	didSet, err := s.storage.SetIfNotExists(key, value)
	if err != nil {
		s.Log.Errorf("%s error setting: %v", s.id, err)
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
	s.Log.Debugf("%s removed key %v", s.id, key)
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
