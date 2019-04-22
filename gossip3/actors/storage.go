package actors

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	lru "github.com/hashicorp/golang-lru"
	"github.com/quorumcontrol/differencedigest/ibf"
	"github.com/quorumcontrol/storage"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
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
	return actor.PropsFromProducer(func() actor.Actor {
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
	}).WithReceiverMiddleware(
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
	case *extmsgs.Store:
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
	case *messages.GzipExport:
		context.Respond(s.GzipExport())
	case *messages.GzipImport:
		context.Respond(s.GzipImport(context, msg.Payload))
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

func (s *Storage) loadIBFAtStart() {
	if err := s.storage.ForEachKey([]byte{}, func(key []byte) error {
		s.addKeyToIBFs(key)
		return nil
	}); err != nil {
		panic(err)
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

func (s *Storage) GzipExport() []byte {
	buf := new(bytes.Buffer)
	w := gzip.NewWriter(buf)
	wroteCount := 0

	s.Log.Debugw("gzipExport started")

	s.storage.ForEach([]byte{}, func(key, value []byte) error {
		wroteCount++
		keyPrefix := make([]byte, 4)
		binary.BigEndian.PutUint32(keyPrefix, uint32(len(key)))
		pairPrefix := make([]byte, 4)
		binary.BigEndian.PutUint32(pairPrefix, uint32(len(keyPrefix)+len(key)+len(value)))

		w.Write(pairPrefix)
		w.Write(keyPrefix)
		w.Write(key)
		w.Write(value)
		return nil
	})
	w.Close()

	if wroteCount == 0 {
		return nil
	}

	s.Log.Debugw("gzipExport exported %d keys", wroteCount)

	return buf.Bytes()
}

func (s *Storage) GzipImport(context actor.Context, payload []byte) uint64 {
	buf := bytes.NewBuffer(payload)
	reader, err := gzip.NewReader(buf)
	if err != nil {
		panic(fmt.Sprintf("Error creating gzip reader %v", err))
	}
	defer reader.Close()

	s.Log.Debugw("gzipImport from %v started", context.Sender())

	var wroteCount uint64

	bytesLeft := true

	for bytesLeft {
		prefix := make([]byte, 4)
		_, err := io.ReadFull(reader, prefix)
		if err != nil {
			panic(fmt.Sprintf("Error reading kv pair length %v", err))
		}
		prefixLength := binary.BigEndian.Uint32(prefix)

		kvPair := make([]byte, int(prefixLength))
		_, err = io.ReadFull(reader, kvPair)
		if err != nil {
			panic(fmt.Sprintf("Error reading kv pair %v", err))
		}

		keyLength := binary.BigEndian.Uint32(kvPair[0:4])
		valueStartIndex := 4 + keyLength

		context.Send(context.Self(), &extmsgs.Store{
			Key:        kvPair[4:valueStartIndex],
			Value:      kvPair[valueStartIndex:],
			SkipNotify: true,
		})

		wroteCount++

		bytesLeft = buf.Len() > 0
	}

	s.Log.Debugw("gzipImport from %v loaded %d keys", context.Sender(), wroteCount)

	return wroteCount
}
