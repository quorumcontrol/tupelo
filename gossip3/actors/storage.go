package actors

import (
	"encoding/binary"
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/differencedigest/ibf"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/storage"
)

type ibfMap map[int]*ibf.InvertibleBloomFilter

var standardIBFSizes = []int{500, 2000, 100000}

var log = middleware.Log

// PushSyncer is the main remote-facing actor that handles
// Sending out syncs
type Storage struct {
	id      string
	ibfs    ibfMap
	storage *storage.MemStorage
	strata  *ibf.DifferenceStrata
}

var StorageProps *actor.Props = actor.FromProducer(NewStorage).WithMiddleware(middleware.LoggingMiddleware)

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
		log.Infow("started", "actor", context.Self().Id)
	case *messages.Debug:
		log.Infof("message: %v", msg.Message)
	case *messages.Store:
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
			log.Errorw("error getting keys", "me", context.Self().GetId(), "err", err)
		}
		context.Respond(keys)
	}
}

func (s *Storage) Add(key, value []byte) (bool, error) {
	didSet, err := s.storage.SetIfNotExists(key, value)
	if err != nil {
		log.Errorf("%s error setting: %v", s.id, err)
		return false, fmt.Errorf("error setting storage: %v", err)
	}
	if didSet {
		ibfObjectID := byteToIBFsObjectId(key[0:8])
		s.strata.Add(ibfObjectID)
		for _, filter := range s.ibfs {
			filter.Add(ibfObjectID)
		}
	} else {
		log.Debugf("%s skipped adding, already exists %v", s.id, key)
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
	log.Debugf("%s removed key %v", s.id, key)
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
