package gossip4

import (
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	"github.com/quorumcontrol/messages/build/go/signatures"
)

type Checkpoint struct {
	CurrentState      cid.Cid // a HAMT
	Height            uint64
	Previous          cid.Cid // the CID of the previous Checkpoint
	PreviousSignature signatures.Signature
	node              *hamt.Node // while local
}

func (c *Checkpoint) Copy() *Checkpoint {
	return &Checkpoint{
		CurrentState:      c.CurrentState,
		Height:            c.Height,
		Previous:          c.Previous,
		PreviousSignature: c.PreviousSignature,
		node:              c.node.Copy(),
	}
}
