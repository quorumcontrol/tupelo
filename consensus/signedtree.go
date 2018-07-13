package consensus

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/chaintree/chaintree"
)

var DefaultTransactors = map[string]chaintree.TransactorFunc{
	"SET_DATA": SetDataTransaction,
	"SET_OWNERSHIP": SetOwnershipTransaction,
}

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

func NewSignedChainTree(key ecdsa.PublicKey) (*SignedChainTree, error) {
	tree, err := chaintree.NewChainTree(
		NewEmptyTree(AddrToDid(crypto.PubkeyToAddress(key).String())),
		nil,
		DefaultTransactors,
	)

	if err != nil {
		return nil, fmt.Errorf("error creating tree: %v", err)
	}

	return &SignedChainTree{
		ChainTree:  tree,
		Signatures: make(SignatureMap),
	}, nil
}
