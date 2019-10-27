package gossip4

import (
	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/messages/build/go/signatures"
)

type Checkpoint struct {
	Transactions      cid.Cid // a datastructure - maybe this is just a big array? or a hamt
	CurrentState      cid.Cid // a HAMT
	Height            uint64
	Previous          cid.Cid // the CID of the previous Checkpoint
	PreviousSignature *signatures.Signature
}
