package gossip4

import (
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/safewrap"
)

func init() {
	cbornode.RegisterCborType(Block{})
}

type Block struct {
	Height       uint64
	Transactions []cid.Cid
	wrapped      *cbornode.Node
}

func (b *Block) ID() string {
	return b.Wrapped().Cid().String()
}

func (b *Block) Wrapped() *cbornode.Node {
	if b.wrapped != nil {
		return b.wrapped
	}
	sw := safewrap.SafeWrap{}
	n := sw.WrapObject(b)
	b.wrapped = n
	return n
}

func (b *Block) Length() int {
	return len(b.Transactions)
}
