package types

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/gossip"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo/sdk/gossip/hamtwrapper"
)

func init() {
	cbornode.RegisterCborType(services.AddBlockRequest{})
	cbornode.RegisterCborType(gossip.Round{})
	cbornode.RegisterCborType(gossip.RoundConfirmation{})
}

type RoundWrapper struct {
	round   *gossip.Round
	wrapped *cbornode.Node

	checkpoint *CheckpointWrapper
	hamtNode   *hamt.Node
	store      nodestore.DagStore
}

func WrapRound(round *gossip.Round) *RoundWrapper {
	sw := safewrap.SafeWrap{}
	node := sw.WrapObject(round)
	return &RoundWrapper{
		round:   round,
		wrapped: node,
	}
}

func (r *RoundWrapper) Value() *gossip.Round {
	return r.round
}

func (r *RoundWrapper) CID() cid.Cid {
	return r.Wrapped().Cid()
}

func (r *RoundWrapper) Wrapped() *cbornode.Node {
	return r.wrapped
}

func (r *RoundWrapper) Height() uint64 {
	return r.round.Height
}

func (r *RoundWrapper) SetStore(store nodestore.DagStore) {
	r.store = store
}

func (r *RoundWrapper) FetchCheckpoint(ctx context.Context) (*CheckpointWrapper, error) {
	if r.checkpoint != nil {
		return r.checkpoint, nil
	}

	if r.store == nil {
		return nil, fmt.Errorf("missing a store on the completed round, use SetStore")
	}

	checkpoint := &gossip.Checkpoint{}
	checkpointCid, err := cid.Cast(r.round.CheckpointCid)
	if err != nil {
		return nil, fmt.Errorf("error casting checkpoint cid: %v", err)
	}
	checkpointNode, err := r.store.Get(ctx, checkpointCid)
	if err != nil {
		return nil, fmt.Errorf("error fetching checkpoint %w", err)
	}
	err = cbornode.DecodeInto(checkpointNode.RawData(), checkpoint)
	if err != nil {
		return nil, fmt.Errorf("error decoding %w", err)
	}
	r.checkpoint = WrapCheckpoint(checkpoint)

	return r.checkpoint, nil
}

func (r *RoundWrapper) FetchHamt(ctx context.Context) (*hamt.Node, error) {
	if r.hamtNode != nil {
		return r.hamtNode, nil
	}

	if r.store == nil {
		return nil, fmt.Errorf("missing a store on the completed round, use SetStore")
	}

	hamtStore := hamtwrapper.DagStoreToCborIpld(r.store)
	stateCid, err := cid.Cast(r.round.StateCid)
	if err != nil {
		return nil, fmt.Errorf("error casting state cid: %v", err)
	}

	n, err := hamt.LoadNode(ctx, hamtStore, stateCid, hamt.UseTreeBitWidth(5))
	if err != nil {
		return nil, fmt.Errorf("error loading hamt %w", err)
	}

	r.hamtNode = n

	return n, nil
}

func WrapRoundConfirmation(conf *gossip.RoundConfirmation) *RoundConfirmationWrapper {
	sw := safewrap.SafeWrap{}
	wrapped := sw.WrapObject(conf)

	return &RoundConfirmationWrapper{
		value:   conf,
		wrapped: wrapped,
	}
}

type RoundConfirmationWrapper struct {
	value   *gossip.RoundConfirmation
	wrapped *cbornode.Node

	completedRound *RoundWrapper
	store          nodestore.DagStore
}

func (rc *RoundConfirmationWrapper) Value() *gossip.RoundConfirmation {
	return rc.value
}

func (rc *RoundConfirmationWrapper) SetStore(store nodestore.DagStore) {
	rc.store = store
}

func (rc *RoundConfirmationWrapper) Height() uint64 {
	return rc.value.Height
}

func (rc *RoundConfirmationWrapper) FetchCompletedRound(ctx context.Context) (*RoundWrapper, error) {
	if rc.completedRound != nil {
		return rc.completedRound, nil
	}

	if rc.store == nil {
		return nil, fmt.Errorf("missing a store on the round confirmation, use SetStore")
	}

	roundCid, err := cid.Cast(rc.value.RoundCid)
	if err != nil {
		return nil, fmt.Errorf("error casting round cid: %v", err)
	}

	roundNode, err := rc.store.Get(ctx, roundCid)
	if err != nil {
		return nil, fmt.Errorf("error getting node: %w", err)
	}

	completedRound := &gossip.Round{}
	err = cbornode.DecodeInto(roundNode.RawData(), completedRound)
	if err != nil {
		return nil, fmt.Errorf("error decoding: %w", err)
	}

	wrappedCompletedRound := WrapRound(completedRound)
	wrappedCompletedRound.SetStore(rc.store)
	rc.completedRound = wrappedCompletedRound

	return wrappedCompletedRound, nil
}

func (rc *RoundConfirmationWrapper) Data() []byte {
	return rc.Wrapped().RawData()
}

func (rc *RoundConfirmationWrapper) Wrapped() *cbornode.Node {
	if rc.wrapped != nil {
		return rc.wrapped
	}
	sw := safewrap.SafeWrap{}
	n := sw.WrapObject(rc)
	rc.wrapped = n
	return n
}
