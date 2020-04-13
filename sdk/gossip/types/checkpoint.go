package types

import (
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/gossip"
)

func init() {
	cbornode.RegisterCborType(gossip.Checkpoint{})
}

func WrapCheckpoint(c *gossip.Checkpoint) *CheckpointWrapper {
	sw := safewrap.SafeWrap{}
	n := sw.WrapObject(c)

	return &CheckpointWrapper{
		value:   c,
		wrapped: n,
	}
}

type CheckpointWrapper struct {
	value   *gossip.Checkpoint
	wrapped *cbornode.Node
}

func (c *CheckpointWrapper) CID() cid.Cid {
	return c.Wrapped().Cid()
}

func (c *CheckpointWrapper) Value() *gossip.Checkpoint {
	return c.value
}

func (c *CheckpointWrapper) Wrapped() *cbornode.Node {
	return c.wrapped
}

func (c *CheckpointWrapper) Length() int {
	return len(c.value.AddBlockRequests)
}

func (c *CheckpointWrapper) AddBlockRequests() [][]byte {
	return c.value.AddBlockRequests
}
