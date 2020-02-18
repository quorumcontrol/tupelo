package gossip

import (
	"sync"

	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
)

type gsInflightHolder map[cid.Cid]*AddBlockWrapper

type globalState struct {
	sync.RWMutex

	hamt     *hamt.Node
	inflight gsInflightHolder
}

func newGlobalState(store *hamt.CborIpldStore) *globalState {
	return &globalState{
		hamt:     hamt.NewNode(store, hamt.UseTreeBitWidth(5)),
		inflight: make(gsInflightHolder),
	}
}

func (gs *globalState) Copy() *globalState {
	gs.RLock()
	defer gs.RUnlock()

	newInflight := make(gsInflightHolder)
	for k, v := range gs.inflight {
		newInflight[k] = v
	}

	return &globalState{
		hamt:     gs.hamt.Copy(),
		inflight: newInflight,
	}
}
