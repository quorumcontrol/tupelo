package api

import (
	"context"
	"encoding/base64"
	"fmt"

	format "github.com/ipfs/go-ipld-format"

	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo/aggregator"
	"github.com/quorumcontrol/tupelo/sdk/gossip/types"
)

type Resolver struct {
	aggregator *aggregator.Aggregator
}

func NewResolver(ctx context.Context) (*Resolver, error) {
	ng := types.NewNotaryGroup("aggregator")
	agg, err := aggregator.NewAggregator(ctx, aggregator.NewMemoryStore(), ng)
	if err != nil {
		return nil, fmt.Errorf("error creating aggregator: %w", err)
	}
	return &Resolver{
		aggregator: agg,
	}, nil
}

type ResolveInput struct {
	Input struct {
		Did  string
		Path string
	}
}

type resolvePayloadResolver struct {
}

func (r *resolvePayloadResolver) Value() *JSON {
	return &JSON{Object: "hi"}
}

func (r *resolvePayloadResolver) RemainingPath() []string {
	empty := []string{}
	return empty
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

func (r *Resolver) Resolve(ctx context.Context, input ResolveInput) (*resolvePayloadResolver, error) {
	return &resolvePayloadResolver{}, nil
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

	resp, err := r.aggregator.Add(ctx, abr)
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
