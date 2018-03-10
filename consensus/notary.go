package consensus

import (
	"github.com/quorumcontrol/qc3/internalchain"
	"github.com/quorumcontrol/qc3/bls"
	"log"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"fmt"
	"context"
)

type Storage interface {
	Set(id string, chain *internalchain.InternalChain) (error)
	Get(id string) (*internalchain.InternalChain,error)
}

type Notary struct {
	ChainStore Storage
	VerKey *bls.VerKey
	SignKey *bls.SignKey
	Validators []ValidatorFunc
}

func NewNotary(storage Storage, signKey *bls.SignKey) *Notary {
	verKey,err := signKey.VerKey()
	if err != nil {
		log.Panicf("error getting verkey from sign key: %v", err)
	}
	return &Notary{
		ChainStore: storage,
		SignKey: signKey,
		VerKey: verKey,
		Validators: []ValidatorFunc{
			IsValidOwnerSig,
		},
	}
}

func (n *Notary) CanSignBlock(ctx context.Context, block *consensuspb.Block) (bool,error) {
	currentChain,err := n.ChainStore.Get(block.SignableBlock.ChainId)
	if err != nil {
		return false, fmt.Errorf("error getting existing chain: %v", err)
	}

	ctx = context.WithValue(ctx, "storage", n.ChainStore)

	for _,validatatorFunc := range n.Validators {
		isValid,err := validatatorFunc(ctx, currentChain, block)
		if err != nil {
			return false, fmt.Errorf("error getting validation: %v: %v", validatatorFunc, err)
		}
		if !isValid {
			return false, nil
		}
	}

	return false, nil
}


