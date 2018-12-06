package gossip2

import (
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
)

// GossipReporter is an interface for providing metrics
// on the actual gossip process
type GossipReporter interface {
	Seen(seer, sender string, id []byte, when time.Time) error
	Processed(seer, sender string, id []byte, when time.Time) error
	GetSeenEventsFor(id []byte) IDEntries
}

type InMemoryReporter struct {
	locker          *sync.RWMutex
	seenEntries     rootEntries
	procesedEntries rootEntries
}

type Event struct {
	Seer   string
	Sender string
	Id     []byte
	When   time.Time
}

type IDEntries map[string]Event
type rootEntries map[string]IDEntries

func NewInMemoryReporter() *InMemoryReporter {
	return &InMemoryReporter{
		locker:          new(sync.RWMutex),
		seenEntries:     make(rootEntries),
		procesedEntries: make(rootEntries),
	}
}

func (imr *InMemoryReporter) Seen(seer, sender string, id []byte, when time.Time) error {
	hexID := hexutil.Encode(id)
	imr.locker.Lock()
	forIDs, ok := imr.seenEntries[hexID]
	if !ok {
		forIDs = make(IDEntries)
	}
	evt := Event{
		Seer:   seer,
		Sender: sender,
		Id:     id,
		When:   when,
	}
	forIDs[seer] = evt
	imr.seenEntries[hexID] = forIDs
	imr.locker.Unlock()
	return nil
}

func (imr *InMemoryReporter) Processed(seer, sender string, id []byte, when time.Time) error {
	hexID := hexutil.Encode(id)
	imr.locker.Lock()
	forIDs, ok := imr.procesedEntries[hexID]
	if !ok {
		forIDs = make(IDEntries)
	}
	forIDs[seer] = Event{
		Seer:   seer,
		Sender: sender,
		Id:     id,
		When:   when,
	}
	imr.locker.Unlock()
	return nil
}

func (imr *InMemoryReporter) GetSeenEventsFor(id []byte) IDEntries {
	hexID := hexutil.Encode(id)
	imr.locker.RLock()
	defer imr.locker.RUnlock()
	return imr.seenEntries[hexID]
}

// Default is to do nothing
type NoOpReporter struct{}

func (noor *NoOpReporter) Seen(seer, sender string, id []byte, when time.Time) error {
	return nil
}

func (noor *NoOpReporter) Processed(seer, sender string, id []byte, when time.Time) error {
	return nil
}

func (noor *NoOpReporter) GetSeenEventsFor(id []byte) IDEntries {
	return nil
}
