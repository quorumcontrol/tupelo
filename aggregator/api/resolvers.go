package api

import (
	"context"

	"github.com/ipfs/go-cid"
)

type Resolver struct {
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

func (r *Resolver) AddBlock(ctx context.Context, input AddBlockInput) (*AddBlockPayload, error) {
	return &AddBlockPayload{
		Valid:  false,
		NewTip: cid.Undef.String(),
	}, nil
}
