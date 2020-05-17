package aggregator

import (
	"context"
	"fmt"
	"sync"

	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	"github.com/opentracing/opentracing-go"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/services"
)

// map of objectID to latest AddBLockWrapper
type gsInflightHolder map[string]*services.AddBlockRequest

func (gifh gsInflightHolder) Copy() gsInflightHolder {
	newInflight := make(gsInflightHolder, len(gifh))
	for k, v := range gifh {
		newInflight[k] = v
	}
	return newInflight
}

type globalState struct {
	sync.RWMutex

	hamt     *hamt.Node
	store    *hamt.CborIpldStore
	inflight gsInflightHolder
}

func newGlobalState(store *hamt.CborIpldStore) *globalState {
	return &globalState{
		hamt:     hamt.NewNode(store, hamt.UseTreeBitWidth(5)),
		inflight: make(gsInflightHolder),
		store:    store,
	}
}

func (gs *globalState) Copy() *globalState {
	gs.RLock()
	defer gs.RUnlock()

	return &globalState{
		hamt:     gs.hamt.Copy(),
		inflight: gs.inflight.Copy(),
		store:    gs.store,
	}
}

func (gs *globalState) Add(abrws ...*services.AddBlockRequest) {
	gs.Lock()
	defer gs.Unlock()

	for _, abrw := range abrws {
		gs.inflight[string(abrw.ObjectId)] = abrw
	}
}

func (gs *globalState) Find(ctx context.Context, objectID string) (*services.AddBlockRequest, error) {
	sp := opentracing.StartSpan("gossip4.globalState.Find")
	defer sp.Finish()

	gs.RLock()
	defer gs.RUnlock()
	if gs.hamt == nil {
		return nil, nil
	}

	abr, ok := gs.inflight[objectID]
	if ok {
		return abr, nil
	}

	var abrCid cid.Cid

	err := gs.hamt.Find(ctx, objectID, &abrCid)
	if err != nil {
		if err == hamt.ErrNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting abrCID: %v", err)
	}

	if !abrCid.Equals(cid.Undef) {
		err = gs.store.Get(ctx, abrCid, abr)
		if err != nil {
			return nil, fmt.Errorf("error getting abr: %v", err)
		}
	}
	return abr, nil
}

func (gs *globalState) Process(ctx context.Context) error {
	sp := opentracing.StartSpan("gossip4.globalState.backgroundProcess")
	defer sp.Finish()

	gs.RLock()
	inflightCopy := gs.inflight.Copy()
	gs.RUnlock()

	sw := safewrap.SafeWrap{}

	txSp := opentracing.StartSpan("gossip4.globalState.backgroundProcess.txs", opentracing.ChildOf(sp.Context()))
	for objectID, abr := range inflightCopy {
		wrapped := sw.WrapObject(abr)
		if sw.Err != nil {
			return fmt.Errorf("error serializing: %w", sw.Err)
		}

		gs.Lock()
		err := gs.hamt.Set(ctx, objectID, wrapped.Cid())
		if err != nil {
			gs.Unlock()
			return fmt.Errorf("error setting hamt: %w", err)
		}
		delete(gs.inflight, objectID)
		gs.Unlock()
	}
	txSp.Finish()

	flushSp := opentracing.StartSpan("gossip4.globalState.backgroundProcess.flush", opentracing.ChildOf(sp.Context()))
	err := gs.hamt.Flush(ctx)
	if err != nil {
		panic(fmt.Errorf("error flushing rootNode: %w", err))
	}
	flushSp.Finish()

	return nil
}
