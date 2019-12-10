package gossip4

import (
	"sync"

	"github.com/ipfs/go-hamt-ipld"
)

const (
	defaultAlpha = 0.8
	defaultBeta  = 150
	defaultK     = 10
)

type round struct {
	snowball *Snowball
	height   uint64
	state    *hamt.Node
}

func newRound(height uint64) *round {
	return &round{
		height:   height,
		snowball: NewSnowball(defaultAlpha, defaultBeta, defaultK),
	}
}

type roundHolder struct {
	sync.RWMutex
	currentRound uint64
	rounds       map[uint64]*round
}

func newRoundHolder() *roundHolder {
	return &roundHolder{
		rounds: make(map[uint64]*round),
	}
}

func (rh *roundHolder) Current() *round {
	rh.RLock()
	r := rh.rounds[rh.currentRound]
	rh.RUnlock()
	return r
}

func (rh *roundHolder) Get(height uint64) (*round, bool) {
	rh.RLock()
	r, ok := rh.rounds[height]
	rh.RUnlock()
	return r, ok
}

func (rh *roundHolder) SetCurrent(height uint64, r *round) {
	rh.Lock()
	rh.rounds[height] = r
	rh.currentRound = height
	rh.Unlock()
}
