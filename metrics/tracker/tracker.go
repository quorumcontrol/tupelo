package tracker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	cbornode "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/messages/v2/build/go/gossip"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/client"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/hamtwrapper"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/types"
	"github.com/quorumcontrol/tupelo/metrics/result"
)

var log = logging.Logger("tupelo.metrics.tracker")

var defaultClassification = "default"

type Tracker struct {
	tupelo      *client.Client
	nodestore   nodestore.DagStore
	classifier  ClassifierFunc
	recorder    Recorder
	concurrency int
	mux         sync.Mutex
}

type Options struct {
	Tupelo     *client.Client
	Nodestore  nodestore.DagStore
	Classifier ClassifierFunc
	Recorder   Recorder
}

type Recorder interface {
	Start(ctx context.Context, startRound *types.RoundWrapper, endRound *types.RoundWrapper) error
	Record(ctx context.Context, res *result.Result) error
	Finish(ctx context.Context) error
}

type ClassifierFunc func(ctx context.Context, startDag *dag.Dag, endDag *dag.Dag) (classification string, tags map[string]interface{}, err error)

var emptyCid = cid.Cid{}

func New(opts *Options) *Tracker {
	return &Tracker{
		tupelo:      opts.Tupelo,
		nodestore:   opts.Nodestore,
		classifier:  opts.Classifier,
		recorder:    opts.Recorder,
		concurrency: 20,
	}
}

func (t *Tracker) TrackAll(ctx context.Context) error {
	return t.TrackFrom(ctx, emptyCid)
}

func (t *Tracker) TrackFrom(ctx context.Context, startRoundCid cid.Cid) error {
	log.Infof("fetching current round")

	err := t.tupelo.WaitForFirstRound(ctx, 30*time.Second)
	if err != nil {
		return err
	}
	roundWrap := t.tupelo.CurrentRound()

	log.Infof("current round is %d", roundWrap.Height())

	ctx2, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	round, err := roundWrap.FetchCompletedRound(ctx2)
	if err != nil {
		return err
	}
	log.Infof("current round cid is %s", round.CID().String())

	return t.TrackBetween(ctx, startRoundCid, round.CID())
}

func (t *Tracker) emptyRound(ctx context.Context) (*types.RoundWrapper, error) {
	dagStore := nodestore.MustMemoryStore(ctx)
	hamtStore := hamtwrapper.DagStoreToCborIpld(dagStore)
	emptyHamt := hamt.NewNode(hamtStore, hamt.UseTreeBitWidth(5))
	stateCid, err := hamtStore.Put(ctx, emptyHamt)
	if err != nil {
		return nil, err
	}

	round := types.WrapRound(&gossip.Round{
		Height:        0,
		StateCid:      stateCid.Bytes(),
		CheckpointCid: nil,
	})
	round.SetStore(dagStore)
	return round, nil
}

func (t *Tracker) getTreeFromHamt(ctx context.Context, h *hamt.Node, did string) (*dag.Dag, error) {
	ctx2, cancelFn := context.WithTimeout(ctx, 3*time.Second)
	defer cancelFn()

	abrCid := &cid.Cid{}

	err := h.Find(ctx2, did, abrCid)
	if err == hamt.ErrNotFound {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("error finding %s in hamt: %v", did, err)
	}

	abrNode, err := t.nodestore.Get(ctx2, *abrCid)
	if err != nil {
		return nil, err
	}

	abr := &services.AddBlockRequest{}
	err = cbornode.DecodeInto(abrNode.RawData(), abr)
	if err != nil {
		return nil, fmt.Errorf("error decoding ABR: %w", err)
	}

	newTip, err := cid.Cast(abr.NewTip)
	if err != nil {
		return nil, err
	}

	return dag.NewDag(ctx2, newTip, t.nodestore), nil
}

func (t *Tracker) TrackBetween(ctx context.Context, startRoundCid cid.Cid, endRoundCid cid.Cid) error {
	var err error
	var startRound *types.RoundWrapper

	log.Infof("gathering metrics for ng '%s'", t.tupelo.Group.ID)

	if endRoundCid.Equals(startRoundCid) {
		return fmt.Errorf("can't calculate metrics between the same round")
	}

	if startRoundCid.Equals(emptyCid) {
		startRound, err = t.emptyRound(ctx)
	} else {
		startRound, err = t.fetchRound(ctx, startRoundCid)
	}
	if err != nil {
		return err
	}

	endRound, err := t.fetchRound(ctx, endRoundCid)
	if err != nil {
		return err
	}

	log.Infof("measuring between round %d and round %d", startRound.Height(), endRound.Height())

	endHamt, err := endRound.FetchHamt(ctx)
	if err != nil {
		return err
	}

	startHamt, err := startRound.FetchHamt(ctx)
	if err != nil {
		return err
	}

	err = t.recorder.Start(ctx, startRound, endRound)
	if err != nil {
		return err
	}

	errors := make(map[string]error)

	sem := make(chan bool, t.concurrency)

	endHamt.ForEach(ctx, func(did string, _ interface{}) error {
		sem <- true

		go func(did string) {
			defer func() { <-sem }()

			result, err := t.calculateResult(ctx, startHamt, endHamt, did)
			result.Round = endRound.Height()
			result.LastRound = startRound.Height()

			t.mux.Lock()
			defer t.mux.Unlock()

			if err != nil {
				result.Tags["err"] = true
				errors[did] = err
			}

			t.recorder.Record(ctx, result)
		}(did)

		return nil
	})

	// fill back up ch to make sure all fetches have finished
	for i := 0; i < cap(sem); i++ {
		sem <- true
	}

	for did, err := range errors {
		log.Errorf("error measuring %s: %v", did, err)
	}

	return t.recorder.Finish(ctx)
}

func (t *Tracker) calculateResult(ctx context.Context, startHamt *hamt.Node, endHamt *hamt.Node, did string) (*result.Result, error) {
	log.Infof("calculating result for did: %s", did)

	r := &result.Result{
		Did:  did,
		Type: defaultClassification,
		Tags: make(map[string]interface{}),
	}

	endDag, err := t.getTreeFromHamt(ctx, endHamt, did)
	if err != nil {
		return r, err
	}
	log.Debugf("did: %s  endingTip: %s", did, endDag.Tip.String())

	startDag, err := t.getTreeFromHamt(ctx, startHamt, did)
	if err != nil && err != hamt.ErrNotFound {
		return r, err
	}
	err = nil

	var startHeight uint64
	if startDag != nil {
		log.Debugf("did: %s  startingTip: %s", did, startDag.Tip.String())
		startHeight, err = height(ctx, startDag)
		if err != nil {
			return r, fmt.Errorf("error fetching height from startDag: %v", err)
		}
	}

	r.TotalBlocks, err = height(ctx, endDag)
	if err != nil {
		return r, fmt.Errorf("error fetching height from endDag: %v", err)
	}
	r.DeltaBlocks = r.TotalBlocks - startHeight
	log.Debugf("did: %s  totalBlocks: %d  deltaBlocks: %d", did, r.TotalBlocks, r.DeltaBlocks)

	classification, tags, err := t.classifier(ctx, startDag, endDag)
	if classification != "" {
		r.Type = classification
	}
	if tags != nil {
		r.Tags = mergeMaps(r.Tags, tags)
	}
	log.Debugf("did: %s  type: %s  tags: %v", did, r.Type, r.Tags)
	if err != nil {
		return r, fmt.Errorf("error on classification: %v", err)
	}

	return r, nil
}

func (t *Tracker) fetchRound(ctx context.Context, round cid.Cid) (*types.RoundWrapper, error) {
	roundNode, err := t.nodestore.Get(ctx, round)
	if err != nil {
		return nil, fmt.Errorf("error getting node: %w", err)
	}

	r := &gossip.Round{}
	err = cbornode.DecodeInto(roundNode.RawData(), r)
	if err != nil {
		return nil, fmt.Errorf("error decoding: %w", err)
	}

	wrappedRound := types.WrapRound(r)
	wrappedRound.SetStore(t.nodestore)
	return wrappedRound, nil
}

func height(ctx context.Context, d *dag.Dag) (uint64, error) {
	ctx2, cancelFn := context.WithTimeout(ctx, 5*time.Second)
	defer cancelFn()

	root := &chaintree.RootNode{}
	err := d.ResolveInto(ctx2, []string{}, root)
	if err != nil {
		return 0, err
	}
	return root.Height, nil
}

func mergeMaps(a map[string]interface{}, b map[string]interface{}) map[string]interface{} {
	new := make(map[string]interface{})

	if a != nil {
		for k, v := range a {
			new[k] = v
		}
	}

	if b != nil {
		for k, v := range b {
			new[k] = v
		}
	}

	return new
}
