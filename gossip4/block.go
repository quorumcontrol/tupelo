package gossip4

import (
	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/services"
)

type Block struct {
	Height         uint64
	Transactions   []cid.Cid
	transactionMap map[string]*services.AddBlockRequest
}

func (b *Block) ID() string {
	sw := safewrap.SafeWrap{}
	n := sw.WrapObject(b)
	return n.Cid().String()
}

func (b *Block) Length() int {
	return len(b.Transactions)
}
