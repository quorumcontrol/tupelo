package gossip

import (
	"context"
	"fmt"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-hamt-ipld"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/opentracing/opentracing-go"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/messages/v2/build/go/gossip"
	"github.com/quorumcontrol/tupelo/sdk/gossip/types"
)

const (
	defaultAlpha = 0.666
	defaultBeta  = 10
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
	currentRound *round
	roundCache   *lru.Cache
	dataStore    datastore.Batching
	dagStore     nodestore.DagStore
	hamtStore    *hamt.CborIpldStore
}

type roundHolderOpts struct {
	DataStore datastore.Batching
	DagStore  nodestore.DagStore
	HamtStore *hamt.CborIpldStore
}

func newRoundHolder(opts *roundHolderOpts) (*roundHolder, error) {
	cache, err := lru.New(64)
	if err != nil {
		return nil, fmt.Errorf("could not create LRU rounds cache: %w", err)
	}

	return &roundHolder{
		roundCache: cache,
		dataStore:  opts.DataStore,
		dagStore:   opts.DagStore,
		hamtStore:  opts.HamtStore,
	}, nil
}

func (rh *roundHolder) Current() *round {
	rh.RLock()
	defer rh.RUnlock()
	return rh.currentRound
}

func (rh *roundHolder) restoreRound(ctx context.Context, height uint64) (*round, error) {
	key := roundKey(height)
	roundCIDBytes, err := rh.dataStore.Get(key)
	if err != nil {
		return nil, fmt.Errorf("could not look up round for height %d: %w", height, err)
	}

	roundCID, err := cid.Cast(roundCIDBytes)
	if err != nil {
		return nil, fmt.Errorf("could not cast to CID: %w", err)
	}

	roundNode, err := rh.dagStore.Get(ctx, roundCID)
	if err != nil {
		return nil, fmt.Errorf("could not get last completed round from DAG store: %w", err)
	}

	var gr gossip.Round
	err = cbornode.DecodeInto(roundNode.RawData(), &gr)
	if err != nil {
		return nil, fmt.Errorf("could not decode round: %w", err)
	}

	rw := types.WrapRound(&gr)
	rw.SetStore(rh.dagStore)

	cp, err := rw.FetchCheckpoint(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not fetch checkpoint from completed round: %w", err)
	}

	gs := newGlobalState(rh.hamtStore)
	h, err := rw.FetchHamt(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not fetch HAMT: %w", err)
	}
	gs.hamt = h

	r := &round{
		height: gr.Height,
		snowball: &Snowball{
			preferred: &Vote{
				Checkpoint: cp,
			},
		},
		state: gs,
	}

	return r, nil
}

func (rh *roundHolder) Get(height uint64) (*round, bool) {
	rh.RLock()
	defer rh.RUnlock()

	if height == rh.currentRound.height {
		return rh.currentRound, true
	}

	uncastRound, ok := rh.roundCache.Get(height)
	if ok {
		return uncastRound.(*round), ok
	}

	r, err := rh.restoreRound(context.TODO(), height)
	if err != nil {
		return nil, false
	}

	rh.roundCache.Add(height, r)

	return r, true
}

func (rh *roundHolder) SetCurrent(r *round) {
	rh.Lock()
	defer rh.Unlock()

	// add the soon-to-be previous round to the LRU cache b/c we'll likely get
	// it soon and this prevents race conditions with storing it
	if rh.currentRound != nil {
		rh.roundCache.Add(rh.currentRound.height, rh.currentRound)
	}

	rh.currentRound = r
}
