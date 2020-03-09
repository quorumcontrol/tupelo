package gossip

import (
	"sync"

	"github.com/opentracing/opentracing-go"
)

const (
	defaultAlpha = 0.666
	defaultBeta  = 5
	defaultK     = 100
)

type round struct {
	snowball *Snowball
	height   uint64
	state    *globalState
	sp       opentracing.Span

	published bool
}

func newRound(height uint64, alpha float64, beta int, k int) *round {
	if alpha == 0.0 {
		alpha = defaultAlpha
	}

	if beta == 0 {
		beta = defaultBeta
	}

	if k == 0 {
		k = defaultK
	}

	sp := opentracing.StartSpan("gossip4.round")
	sp.SetTag("height", height)
	sp.SetTag("alpha", alpha)
	sp.SetTag("beta", beta)
	sp.SetTag("k", k)

	return &round{
		height:   height,
		snowball: NewSnowball(alpha, beta, k),
		sp:       sp,
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

func (rh *roundHolder) SetCurrent(r *round) {
	rh.Lock()
	rh.rounds[r.height] = r
	rh.currentRound = r.height
	rh.Unlock()
}
