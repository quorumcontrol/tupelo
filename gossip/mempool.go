package gossip

import (
	"strconv"
	"sync"

	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/messages/v2/build/go/services"
)

type mempoolConflictSetID string
type conflictSet struct {
	ids       []cid.Cid
	preferred cid.Cid
}

func (cs *conflictSet) Add(id cid.Cid) {
	var preferred cid.Cid
	newCIDString := id.String()
	for _, existingID := range cs.ids {
		if newCIDString < existingID.String() {
			preferred = id
		}
	}

	if preferred.Equals(cid.Undef) {
		preferred = id
	}

	cs.ids = append(cs.ids, id)
	cs.preferred = preferred
}

type mempool struct {
	sync.RWMutex
	// abrs is the map of all *valid* add block requests (including conflicting ones)
	abrs map[cid.Cid]*services.AddBlockRequest
	// conflictSets holds references to ABRs that are in conflict with each other
	// and keeps track of which one is the current preferred
	// the preferred is use to determine what block to propose, but we need to keep
	// the conflicts around in case the other nodes haven't seen our preferred
	// when we delete, we want to delete all conflicting because that's a resolution
	conflictSets map[mempoolConflictSetID]*conflictSet
}

func newMempool() *mempool {
	return &mempool{
		abrs:         make(map[cid.Cid]*services.AddBlockRequest),
		conflictSets: make(map[mempoolConflictSetID]*conflictSet),
	}
}

func (m *mempool) Add(id cid.Cid, abr *services.AddBlockRequest) {
	m.Lock()
	indexKey := toConflictSetID(abr)
	cs, ok := m.conflictSets[indexKey]
	if !ok {
		cs = &conflictSet{}
	}
	cs.Add(id)
	m.conflictSets[indexKey] = cs

	m.abrs[id] = abr
	m.Unlock()
}

func (m *mempool) Get(id cid.Cid) *services.AddBlockRequest {
	m.RLock()
	defer m.RUnlock()
	return m.abrs[id]
}

// DeleteIDAndConflictSet deletes not only this id, but all other
// conflicting ABRs as well.
func (m *mempool) DeleteIDAndConflictSet(id cid.Cid) {
	m.Lock()
	existing, ok := m.abrs[id]
	if ok {
		delete(m.abrs, id)
		csID := toConflictSetID(existing)
		cs, ok := m.conflictSets[csID]
		if ok {
			for _, id := range cs.ids {
				delete(m.abrs, id)
			}
			delete(m.conflictSets, csID)
		}
	}
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

func (m *mempool) Preferred() []cid.Cid {
	m.RLock()
	defer m.RUnlock()

	preferred := make([]cid.Cid, len(m.conflictSets))

	i := 0
	for _, cs := range m.conflictSets {
		preferred[i] = cs.preferred
		i++
	}
	return preferred
}

func (m *mempool) Length() int {
	m.RLock()
	defer m.RUnlock()
	return len(m.abrs)
}

func toConflictSetID(abr *services.AddBlockRequest) mempoolConflictSetID {
	return mempoolConflictSetID(string(abr.ObjectId) + strconv.FormatUint(abr.Height, 10))
}
