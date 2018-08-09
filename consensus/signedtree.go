package consensus

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/chaintree/chaintree"
)

const (
	TransactionTypeSetData      = "SET_DATA"
	TransactionTypeSetOwnership = "SET_OWNERSHIP"
	TransactionTypeStake        = "STAKE"
)

var DefaultTransactors = map[string]chaintree.TransactorFunc{
	TransactionTypeSetData:      SetDataTransaction,
	TransactionTypeSetOwnership: SetOwnershipTransaction,
	TransactionTypeStake:        StakeTransaction,
}

type SignedChainTree struct {
	ChainTree  *chaintree.ChainTree
	Signatures SignatureMap
}

func (sct *SignedChainTree) Id() (string, error) {
	return sct.ChainTree.Id()
}

func (sct *SignedChainTree) MustId() string {
	id, err := sct.ChainTree.Id()
	if err != nil {
		log.Error("error getting id from chaintree: %v", id)
	}
	return id
}

func (sct *SignedChainTree) Tip() *cid.Cid {
	return sct.ChainTree.Dag.Tip
}

func (sct *SignedChainTree) IsGenesis() bool {
	newEmpty := NewEmptyTree(sct.MustId())
	return newEmpty.Tip.Equals(sct.Tip())
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
