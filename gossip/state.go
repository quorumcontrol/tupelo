package gossip

import (
	"context"
	"fmt"
	"sync"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	"github.com/opentracing/opentracing-go"
	"github.com/quorumcontrol/messages/v2/build/go/services"
)

// map of objectID to latest AddBLockWrapper
type gsInflightHolder map[string]*AddBlockWrapper

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

func (gs *globalState) addInflights(abrws ...*AddBlockWrapper) {
	gs.Lock()
	defer gs.Unlock()

	for _, abrw := range abrws {
		gs.inflight[string(abrw.ObjectId)] = abrw
	}
}

func (gs *globalState) Find(ctx context.Context, objectID string) (*services.AddBlockRequest, error) {
	gs.RLock()
	defer gs.RUnlock()
	if gs.hamt == nil {
		return nil, nil
	}

	wrapper, ok := gs.inflight[objectID]
	if ok {
		return wrapper.AddBlockRequest, nil
	}

	var abrCid cid.Cid

	err := gs.hamt.Find(ctx, objectID, &abrCid)
	if err != nil {
		if err == hamt.ErrNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting abrCID: %v", err)
	}

	abr := &services.AddBlockRequest{}

	if !abrCid.Equals(cid.Undef) {
		err = gs.store.Get(ctx, abrCid, abr)
		if err != nil {
			return nil, fmt.Errorf("error getting abr: %v", err)
		}
	}
	return abr, nil
}

func (gs *globalState) backgroundProcess(ctx context.Context, n *Node, round *round) error {
	sp := opentracing.StartSpan("gossip4.backgroundProcess")
	sp.SetTag("round", round.height)
	defer sp.Finish()

	n.logger.Infof("backgroundProcess start on round %d", round.height)

	gs.RLock()
	inflightCopy := gs.inflight.Copy()
	gs.RUnlock()

	for objectID, abrWrapper := range inflightCopy {
		err := gs.hamt.Set(ctx, objectID, abrWrapper.cid)
		if err != nil {
			return fmt.Errorf("error setting hamt: %w", err)
		}
		gs.Lock()
		delete(gs.inflight, objectID)
		gs.Unlock()

		actor.EmptyRootContext.Send(n.stateStorerPid, &saveTransactionState{ctx: ctx, abr: abrWrapper.AddBlockRequest})
	}

	err := gs.hamt.Flush(ctx)
	if err != nil {
		panic(fmt.Errorf("error flushing rootNode: %w", err))
	}

	// Notify clients of the new checkpoint
	err = n.publishCompletedRound(ctx, round)
	if err != nil {
		n.logger.Errorf("error publishing current round: %v", err)
	}

	n.logger.Infof("backgroundProcess finish on round %d", round.height)

	return nil
}
