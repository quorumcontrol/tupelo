package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strings"

	logging "github.com/ipfs/go-log"

	"github.com/graph-gophers/graphql-go"
	"github.com/ipfs/go-datastore"
	format "github.com/ipfs/go-ipld-format"

	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo/aggregator"
	"github.com/quorumcontrol/tupelo/sdk/gossip/types"
	"github.com/quorumcontrol/tupelo/sdk/reftracking"
)

var logger = logging.Logger("resolver")

type Resolver struct {
	Aggregator *aggregator.Aggregator
}

func NewResolver(ctx context.Context, ds datastore.Batching) (*Resolver, error) {
	ng := types.NewNotaryGroup("aggregator")
	agg, err := aggregator.NewAggregator(ctx, ds, ng)
	if err != nil {
		return nil, fmt.Errorf("error creating aggregator: %w", err)
	}
	ng.DagGetter = agg
	return &Resolver{
		Aggregator: agg,
	}, nil
}

type ResolveInput struct {
	Input struct {
		Did  string
		Path string
	}
}

type ResolvePayload struct {
	Value         *JSON
	RemainingPath []string
	TouchedBlocks *[]Block
}

type AddBlockInput struct {
	Input struct {
		AddBlockRequest string //base64
	}
}

type Block struct {
	Data string      `json:"data"`
	Cid  *graphql.ID `json:"cid"`
}

type AddBlockPayload struct {
	Valid     bool
	NewTip    string
	NewBlocks *[]Block
}

type BlocksPayload struct {
	Blocks []Block
}

type BlocksInput struct {
	Input struct {
		Ids []string
	}
}

func (r *Resolver) Blocks(ctx context.Context, args BlocksInput) (*BlocksPayload, error) {
	stringIds := args.Input.Ids
	ids := make([]cid.Cid, len(stringIds))
	for i, stringId := range stringIds {
		id, err := cid.Decode(stringId)
		if err != nil {
			return nil, fmt.Errorf("error getting CID: %w", err)
		}
		ids[i] = id
	}
	blockCh := r.Aggregator.GetMany(ctx, ids)
	blocks := make([]format.Node, len(stringIds))
	i := 0
	for nodeOption := range blockCh {
		if nodeOption.Err != nil {
			return nil, fmt.Errorf("error fetching: %w", nodeOption.Err)
		}
		blocks[i] = nodeOption.Node
		i++
	}
	return &BlocksPayload{Blocks: blocksToGraphQLBlocks(blocks)}, nil
}

func (r *Resolver) Resolve(ctx context.Context, input ResolveInput) (*ResolvePayload, error) {
	logger.Infof("resolving %s %s", input.Input.Did, input.Input.Path)
	path := strings.Split(strings.TrimPrefix(input.Input.Path, "/"), "/")
	latest, err := r.Aggregator.GetLatest(ctx, input.Input.Did)
	if err == aggregator.ErrNotFound {
		logger.Debugf("resolve %s not found", input.Input.Did)
		return &ResolvePayload{
			RemainingPath: path,
		}, nil
	}
	if err != nil {
		logger.Errorf("error getting latest %s %v", input.Input.Did, err)
		return nil, fmt.Errorf("error getting latest: %w", err)
	}

	trackedTree, tracker, err := reftracking.WrapTree(ctx, latest)
	if err != nil {
		return nil, fmt.Errorf("error creating reference tracker: %v", err)
	}

	val, remain, err := trackedTree.Dag.Resolve(ctx, path)
	if err != nil {
		logger.Errorf("error resolving %s %v", input.Input.Did, err)
		return nil, fmt.Errorf("error resolving: %v", err)
	}

	// Grab the nodes that were actually used:
	touchedNodes, err := tracker.TouchedNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting touched nodes: %w", err)
	}

	blocks := blocksToGraphQLBlocks(touchedNodes)

	return &ResolvePayload{
		RemainingPath: remain,
		Value: &JSON{
			Object: val,
		},
		TouchedBlocks: &blocks,
	}, nil
}

func blocksToGraphQLBlocks(nodes []format.Node) []Block {
	retBlocks := make([]Block, len(nodes))
	for i, node := range nodes {
		id := graphql.ID(node.Cid().String())
		retBlocks[i] = Block{
			Data: base64.StdEncoding.EncodeToString(node.RawData()),
			Cid:  &id,
		}
	}
	return retBlocks
}

func (r *Resolver) AddBlock(ctx context.Context, input AddBlockInput) (*AddBlockPayload, error) {
	abrBits, err := base64.StdEncoding.DecodeString(input.Input.AddBlockRequest)
	if err != nil {
		return nil, fmt.Errorf("error decoding string: %w", err)
	}
	abr := &services.AddBlockRequest{}
	err = abr.Unmarshal(abrBits)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling %w", err)
	}

	log.Printf("addBlock %s", abr.ObjectId)

	resp, err := r.Aggregator.Add(ctx, abr)
	if err == aggregator.ErrInvalidBlock {
		return &AddBlockPayload{
			Valid:  false,
			NewTip: cid.Undef.String(),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error validating block: %w", err)
	}

	newBlocks := blocksToGraphQLBlocks(resp.NewNodes)

	return &AddBlockPayload{
		Valid:     true,
		NewTip:    resp.NewTip.String(),
		NewBlocks: &newBlocks,
	}, nil
}
