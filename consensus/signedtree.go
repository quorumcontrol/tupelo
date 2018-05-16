package consensus

import (
	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/chaintree/chaintree"
)

type SignedChainTree struct {
	ChainTree  *chaintree.ChainTree
	Signatures SignatureMap
}

func (sct *SignedChainTree) Id() (string, error) {
	return sct.ChainTree.Id()
}

func (sct *SignedChainTree) Tip() *cid.Cid {
	return sct.ChainTree.Dag.Tip
}
