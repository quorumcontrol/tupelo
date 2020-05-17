package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"strings"

	"github.com/ipfs/go-datastore"
	format "github.com/ipfs/go-ipld-format"

	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo/aggregator"
	"github.com/quorumcontrol/tupelo/sdk/gossip/types"
)

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
}

type AddBlockInput struct {
	Input struct {
		AddBlockRequest string //base64
	}
}

type Block struct {
	Data string
}

type AddBlockPayload struct {
	Valid     bool
	NewTip    string
	NewBlocks *[]Block
}

func (r *Resolver) Resolve(ctx context.Context, input ResolveInput) (*ResolvePayload, error) {
	log.Printf("resolving %s %s", input.Input.Did, input.Input.Path)
	path := strings.Split(strings.TrimPrefix(input.Input.Path, "/"), "/")
	latest, err := r.Aggregator.GetLatest(ctx, input.Input.Did)
	if err == aggregator.ErrNotFound {
		log.Printf("%s not found", input.Input.Did)
		return &ResolvePayload{
			RemainingPath: path,
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("error getting latest: %w", err)
	}

	val, remain, err := latest.Dag.Resolve(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("error resolving: %v", err)
	}

	return &ResolvePayload{
		RemainingPath: remain,
		Value: &JSON{
			Object: val,
		},
	}, nil
}

func blocksToGraphQLBlocks(nodes []format.Node) []Block {
	retBlocks := make([]Block, len(nodes))
	for i, node := range nodes {
		retBlocks[i] = Block{
			Data: base64.StdEncoding.EncodeToString(node.RawData()),
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
