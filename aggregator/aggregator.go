package aggregator

import (
	"bytes"
	"context"
	"fmt"
	"sync"

	"github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	cbornode "github.com/ipfs/go-ipld-cbor"
	format "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"

	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo/sdk/gossip/types"
	"github.com/quorumcontrol/tupelo/signer/gossip"
)

var logger = logging.Logger("aggregator")

type Aggregator struct {
	sync.RWMutex

	validator *gossip.TransactionValidator
	state     *globalState
	dagStore  nodestore.DagStore
	group     *types.NotaryGroup
}

func NewAggregator(ctx context.Context, hamtStore *hamt.CborIpldStore, dagStore nodestore.DagStore, group *types.NotaryGroup) (*Aggregator, error) {
	validator, err := gossip.NewTransactionValidator(ctx, logger, group, nil) // nil is the actor pid
	if err != nil {
		return nil, err
	}
	return &Aggregator{
		state:     newGlobalState(hamtStore),
		validator: validator,
		dagStore:  dagStore,
		group:     group,
	}, nil
}

func (a *Aggregator) GetLatest(ctx context.Context, objectID string) (*chaintree.ChainTree, error) {
	curr, err := a.state.Find(ctx, objectID)
	if err != nil {
		return nil, fmt.Errorf("error getting latest: %v", err)
	}
	if curr == nil {
		return nil, hamt.ErrNotFound
	}

	tip, err := cid.Cast(curr.NewTip)
	if err != nil {
		return nil, fmt.Errorf("error casting tip %w", err)
	}

	validators, err := a.group.BlockValidators(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting validators: %w", err)
	}

	dag := dag.NewDag(ctx, tip, a.dagStore)
	tree, err := chaintree.NewChainTree(ctx, dag, validators, a.group.Config().Transactions)
	if err != nil {
		return nil, fmt.Errorf("error creating tree: %w", err)
	}
	return tree, nil
}

func (a *Aggregator) Add(ctx context.Context, abr *services.AddBlockRequest) (*gossip.AddBlockWrapper, error) {
	wrapper := &gossip.AddBlockWrapper{
		AddBlockRequest: abr,
	}
	newTip, isValid, newNodes, err := a.validator.ValidateAbr(wrapper)
	if !isValid || err != nil {
		return nil, fmt.Errorf("invalid ABR: %w", err)
	}
	wrapper.AddBlockRequest.NewTip = newTip.Bytes()
	wrapper.NewNodes = newNodes
	a.Lock()
	defer a.Unlock()

	curr, err := a.state.Find(ctx, string(abr.ObjectId))
	if err != nil {
		return nil, fmt.Errorf("error finding current: %w", err)
	}

	if curr != nil && !bytes.Equal(curr.NewTip, abr.PreviousTip) {
		return nil, fmt.Errorf("previous tip did not match existing tip: %s", curr.NewTip)
	}

	a.state.Add(abr)

	// TODO: don't hold the lock while doing IO
	a.storeState(ctx, wrapper)
	return wrapper, nil
}

func (a *Aggregator) storeState(ctx context.Context, wrapper *gossip.AddBlockWrapper) error {
	sw := safewrap.SafeWrap{}
	var stateNodes []format.Node
	abr := wrapper.AddBlockRequest

	for _, nodeBytes := range abr.State {
		stateNode := sw.Decode(nodeBytes)

		stateNodes = append(stateNodes, stateNode)
	}

	if sw.Err != nil {
		logger.Errorf("error decoding abr state: %v", sw.Err)
		return fmt.Errorf("error decoding: %w", sw.Err)
	}

	err := a.dagStore.AddMany(ctx, stateNodes)
	if err != nil {
		logger.Errorf("error storing abr state: %v", err)
		return fmt.Errorf("error adding: %w", err)
	}

	err = a.dagStore.AddMany(ctx, wrapper.NewNodes)
	if err != nil {
		logger.Errorf("error storing abr new nodes: %v", err)
		return fmt.Errorf("error adding new nodes: %w", err)
	}
	return nil
}

func (a *Aggregator) GetAddBlockRequest(ctx context.Context, id cid.Cid) (*services.AddBlockRequest, error) {
	abrNode, err := a.dagStore.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	abr := &services.AddBlockRequest{}
	err = cbornode.DecodeInto(abrNode.RawData(), abr)
	if err != nil {
		return nil, err
	}

	return abr, nil
}
