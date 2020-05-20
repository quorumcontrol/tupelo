package aggregator

import (
	"bytes"
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	format "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"

	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/graftabledag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo/sdk/gossip/types"
	"github.com/quorumcontrol/tupelo/signer/gossip"
)

var logger = logging.Logger("aggregator")
var ErrNotFound = datastore.ErrNotFound
var ErrInvalidBlock = fmt.Errorf("InvalidBlock")
var CacheSize = 100

// type DagGetter interface {
// 	GetTip(ctx context.Context, did string) (*cid.Cid, error)
// 	GetLatest(ctx context.Context, did string) (*chaintree.ChainTree, error)
// }
// assert fulfills the interface at compile time
var _ graftabledag.DagGetter = (*Aggregator)(nil)

type AddResponse struct {
	IsValid  bool
	NewTip   cid.Cid
	NewNodes []format.Node
	Wrapper  *gossip.AddBlockWrapper
}

type Aggregator struct {
	nodestore.DagStore

	validator     *gossip.TransactionValidator
	keyValueStore datastore.Batching
	group         *types.NotaryGroup
}

func NewAggregator(ctx context.Context, keyValueStore datastore.Batching, group *types.NotaryGroup) (*Aggregator, error) {
	validator, err := gossip.NewTransactionValidator(ctx, logger, group, nil) // nil is the actor pid
	if err != nil {
		return nil, err
	}
	dagStore, err := nodestore.FromDatastoreOfflineCached(ctx, keyValueStore, CacheSize)
	if err != nil {
		return nil, err
	}
	return &Aggregator{
		keyValueStore: keyValueStore,
		DagStore:      dagStore,
		validator:     validator,
		group:         group,
	}, nil
}

func (a *Aggregator) GetTip(ctx context.Context, objectID string) (*cid.Cid, error) {
	curr, err := a.keyValueStore.Get(datastore.NewKey(objectID))
	if err != nil {
		if err == ErrNotFound {
			return nil, err
		}
		return nil, fmt.Errorf("error getting latest: %v", err)
	}
	tip, err := cid.Cast(curr)
	if err != nil {
		return nil, fmt.Errorf("error casting tip %w", err)
	}
	logger.Debugf("GetTip %s: %s", objectID, tip.String())
	return &tip, nil
}

func (a *Aggregator) GetLatest(ctx context.Context, objectID string) (*chaintree.ChainTree, error) {
	tip, err := a.GetTip(ctx, objectID)
	if err != nil {
		if err == ErrNotFound {
			return nil, err
		}
		return nil, fmt.Errorf("error getting tip: %w", err)
	}
	logger.Debugf("GetLatest %s: %s", objectID, tip.String())

	validators, err := a.group.BlockValidators(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting validators: %w", err)
	}

	dag := dag.NewDag(ctx, *tip, a.DagStore)
	tree, err := chaintree.NewChainTree(ctx, dag, validators, a.group.Config().Transactions)
	if err != nil {
		return nil, fmt.Errorf("error creating tree: %w", err)
	}
	return tree, nil
}

func (a *Aggregator) Add(ctx context.Context, abr *services.AddBlockRequest) (*AddResponse, error) {
	logger.Debugf("add %s %d", string(abr.ObjectId), abr.Height)
	wrapper := &gossip.AddBlockWrapper{
		AddBlockRequest: abr,
	}
	newTip, isValid, newNodes, err := a.validator.ValidateAbr(wrapper)
	if !isValid {
		return nil, ErrInvalidBlock
	}
	if err != nil {
		return nil, fmt.Errorf("invalid ABR: %w", err)
	}
	wrapper.AddBlockRequest.NewTip = newTip.Bytes()
	wrapper.NewNodes = newNodes

	did := string(abr.ObjectId)

	curr, err := a.GetTip(ctx, did)
	if err != nil && err != ErrNotFound {
		logger.Errorf("error getting tip: %w", err)
		return nil, fmt.Errorf("error getting tip: %w", err)
	}

	if curr != nil && !bytes.Equal(curr.Bytes(), abr.PreviousTip) {
		logger.Debugf("non matching tips: %w", err)
		return nil, fmt.Errorf("previous tip did not match existing tip: %s", curr.String())
	}

	logger.Infof("storing %s (height: %d) new tip: %s", did, abr.Height, newTip.String())
	a.storeState(ctx, wrapper)
	err = a.keyValueStore.Put(datastore.NewKey(did), newTip.Bytes())
	if err != nil {
		return nil, fmt.Errorf("error putting key: %w", err)
	}
	return &AddResponse{
		NewTip:   newTip,
		IsValid:  isValid,
		NewNodes: newNodes,
		Wrapper:  wrapper,
	}, nil
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

	err := a.DagStore.AddMany(ctx, append(stateNodes, wrapper.NewNodes...))
	if err != nil {
		logger.Errorf("error storing abr state: %v", err)
		return fmt.Errorf("error adding: %w", err)
	}

	return nil
}
