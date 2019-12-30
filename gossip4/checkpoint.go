package gossip4

import (
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/safewrap"
)

func init() {
	cbornode.RegisterCborType(Checkpoint{})
}

type Checkpoint struct {
	Height           uint64
	AddBlockRequests []cid.Cid
	wrapped          *cbornode.Node
}

func newCheckpoint(height uint64, txs []cid.Cid) *Checkpoint {
	return &Checkpoint{
		Height:           height,
		AddBlockRequests: txs,
	}
}

func (c *Checkpoint) ID() string {
	return c.Wrapped().Cid().String()
}

func (c *Checkpoint) Wrapped() *cbornode.Node {
	if c.wrapped != nil {
		return c.wrapped
	}
	sw := safewrap.SafeWrap{}
	n := sw.WrapObject(c)
	c.wrapped = n
	return n
}

func (c *Checkpoint) Length() int {
	return len(c.AddBlockRequests)
}
