package gossip4

import (
	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"sync"
)

type mempool struct {
	sync.RWMutex
	abrs map[cid.Cid]*services.AddBlockRequest
}

func newMempool() *mempool {
	return &mempool{
		abrs: make(map[cid.Cid]*services.AddBlockRequest),
	}
}

func (m *mempool) Add(id cid.Cid, abr *services.AddBlockRequest) {
	m.Lock()
	m.abrs[id] = abr
	m.Unlock()
}

func (m *mempool) Get(id cid.Cid) *services.AddBlockRequest {
	m.RLock()
	defer m.RUnlock()
	return m.abrs[id]
}

func (m *mempool) Delete(id cid.Cid) {
	m.Lock()
	delete(m.abrs, id)
	m.Unlock()
}

func (m *mempool) Contains(ids ...cid.Cid) bool {
	m.RLock()
	defer m.RUnlock()
	for _, id := range ids {
		_, ok := m.abrs[id]
		if !ok {
			return false
		}
	}
	return true
}

func (m *mempool) Length() int {
	m.RLock()
	defer m.RUnlock()
	return len(m.abrs)
}

func (m *mempool) Keys() []cid.Cid {
	m.RLock()
	keys := make([]cid.Cid, len(m.abrs))
	i := 0
	for k := range m.abrs {
		keys[i] = k
		i++
	}
	m.RUnlock()
	return keys
}
