package gossip4

// import (
// 	"github.com/ipfs/go-cid"
// 	"github.com/ipfs/go-hamt-ipld"
// 	cbornode "github.com/ipfs/go-ipld-cbor"
// 	"github.com/quorumcontrol/messages/v2/build/go/signatures"
// )

// func init() {
// 	cbornode.RegisterCborType(Checkpoint{})
// }

// type Checkpoint struct {
// 	Transactions      []cid.Cid
// 	CurrentState      cid.Cid // a HAMT pointer
// 	Height            uint64
// 	Previous          cid.Cid // the CID of the previous Checkpoint
// 	PreviousSignature signatures.Signature
// 	Signature         signatures.Signature
// 	node              *hamt.Node // the actual hamt to use when local (not sent on the wire)
// }

// func (c *Checkpoint) Copy() *Checkpoint {
// 	return &Checkpoint{
// 		CurrentState:      c.CurrentState,
// 		Height:            c.Height,
// 		Previous:          c.Previous,
// 		PreviousSignature: c.PreviousSignature,
// 		node:              c.node.Copy(),
// 	}
// }

// func (c *Checkpoint) Equals(other *Checkpoint) bool {
// 	return c.CurrentState.Equals(other.CurrentState) &&
// 		c.Height == other.Height &&
// 		c.Previous.Equals(other.Previous) // purposely don't compare signatures
// }

// func (c *Checkpoint) SignerCount() int {
// 	var signerCount int
// 	for _, cnt := range c.Signature.Signers {
// 		if cnt > 0 {
// 			signerCount++
// 		}
// 	}
// 	return signerCount
// }
