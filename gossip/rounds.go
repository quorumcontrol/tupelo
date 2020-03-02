package gossip

import (
	"fmt"
	"strconv"
	"strings"
	"sync"

	lru "github.com/hashicorp/golang-lru"
	"github.com/ipfs/go-datastore"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/opentracing/opentracing-go"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/gossip"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/types"
)

const (
	defaultAlpha = 0.666
	defaultBeta  = 5
	defaultK     = 100

	roundsKeyRoot   = "/Snowball/Rounds"
	currentRoundKey = "/Snowball/CurrentRoundHeight"
)

type round struct {
	snowball *Snowball
	height   uint64
	state    *globalState
	sp       opentracing.Span
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
	currentHeight uint64
	roundCache    *lru.Cache
	store         datastore.Batching
}

func newRoundHolder(store datastore.Batching) *roundHolder {
	currentRoundDSKey := datastore.NewKey(currentRoundKey)
	currentRoundExists, err := store.Has(currentRoundDSKey)
	if err != nil {
		panic(fmt.Errorf("error checking for current round from datastore: %w", err))
	}

	cache, err := lru.New(64)

	if !currentRoundExists {
		return &roundHolder{
			currentHeight: 0,
			roundCache:    cache,
			store:         store,
		}
	}

	crBytes, err := store.Get(currentRoundDSKey)
	if err != nil {
		panic(fmt.Errorf("error getting current round height from datastore: %w", err))
	}

	var cr uint64
	err = cbornode.DecodeInto(crBytes, &cr)
	if err != nil {
		panic(fmt.Errorf("error decoding current round: %w", err))
	}

	return &roundHolder{
		currentHeight: cr,
		roundCache:    cache,
		store:         store,
	}
}

func datastoreKeyForHeight(height uint64) datastore.Key {
	keyStr := strings.Join([]string{roundsKeyRoot, strconv.FormatUint(height, 10)}, "/")
	return datastore.NewKey(keyStr)
}

func (rh *roundHolder) Current() *round {
	rh.Lock()
	defer rh.Unlock()

	cr, _ := rh.get(rh.currentHeight)
	return cr
}

func (rh *roundHolder) Get(height uint64) (*round, bool) {
	rh.RLock()
	defer rh.RUnlock()

	return rh.get(height)
}

// Only for use when already inside a read lock
func (rh *roundHolder) get(height uint64) (*round, bool) {
	uncastRound, ok := rh.roundCache.Get(rh.currentHeight)
	if ok {
		return uncastRound.(*round), true
	}

	key := datastoreKeyForHeight(height)

	ok, err := rh.store.Has(key)
	if err != nil {
		panic(fmt.Errorf("failed to check datastore for key %s: %w", key, err))
	}
	if !ok {
		return nil, ok
	}

	grBytes, err := rh.store.Get(key)
	if err != nil {
		panic(fmt.Errorf("failed to get value from datastore for key %s: %w", key, err))
	}

	var gr gossip.Round
	err = cbornode.DecodeInto(grBytes, &gr)

	// TODO: Do we need to persist / restore anything more than height?
	//  Can likely simplify this further if not.
	r := newRound(gr.Height, 0.0, 0, 0)
	rh.roundCache.Add(height, r)
	return r, true
}

func (rh *roundHolder) SetCurrent(r *round) {
	rh.Lock()
	defer rh.Unlock()

	gRound := &gossip.Round{
		Height: r.height,
	}

	wrappedRound := types.WrapRound(gRound)
	err := rh.store.Put(datastoreKeyForHeight(r.height), wrappedRound.Wrapped().RawData())
	if err != nil {
		panic(fmt.Errorf("unable to store current round: %w", err))
	}

	// Should this even be CBOR?
	sw := safewrap.SafeWrap{}
	wrappedHeight := sw.WrapObject(r.height)
	if sw.Err != nil {
		panic(fmt.Errorf("unable to wrap current height: %w", sw.Err))
	}

	err = rh.store.Put(datastore.NewKey(currentRoundKey), wrappedHeight.RawData())
	if err != nil {
		panic(fmt.Errorf("unable to store current height: %w", err))
	}

	rh.roundCache.Add(r.height, r)

	rh.currentHeight = r.height
}
