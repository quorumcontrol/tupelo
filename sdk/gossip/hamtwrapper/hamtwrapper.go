package hamtwrapper

import (
	"context"
	"fmt"
	"time"

	format "github.com/ipfs/go-ipld-format"

	block "github.com/ipfs/go-block-format"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	"github.com/quorumcontrol/chaintree/nodestore"
)

var hamtAddTimeout = 10 * time.Second

// dsWrapper implements the blocks interface needed for the go-hamt-ipld
// type blocks interface {
//	GetBlock(context.Context, cid.Cid) (block.Block, error)
//	AddBlock(block.Block) error
// }
type dsWrapper struct {
	store nodestore.DagStore
}

func NewStore(underlying nodestore.DagStore) *dsWrapper {
	return &dsWrapper{
		store: underlying,
	}
}

func (dw *dsWrapper) GetBlock(ctx context.Context, id cid.Cid) (block.Block, error) {
	return dw.store.Get(ctx, id)
}

func (dw *dsWrapper) AddBlock(blk block.Block) error {
	ctx, cancel := context.WithTimeout(context.TODO(), hamtAddTimeout)
	defer cancel()
	nd, err := format.Decode(blk) // this can probably be made an unnecessary step
	if err != nil {
		return fmt.Errorf("error decoding block: %w", err)
	}
	return dw.store.Add(ctx, nd)
}

func DagStoreToCborIpld(store nodestore.DagStore) *hamt.CborIpldStore {
	return &hamt.CborIpldStore{
		Blocks: &dsWrapper{store: store},
	}
}
