package notary

import (
	"github.com/quorumcontrol/qc3/internalchain"
	"github.com/quorumcontrol/qc3/bls"
	"log"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"fmt"
	"context"
	"github.com/ethereum/go-ethereum/common"
	"github.com/quorumcontrol/qc3/consensus"
)

type Storage interface {
	Set(id string, chain *internalchain.InternalChain) (error)
	Get(id string) (*internalchain.InternalChain,error)
}

type Signer struct {
	GroupId string
	ChainStore Storage
	VerKey *bls.VerKey
	SignKey *bls.SignKey
	Validators []ValidatorFunc
}

func NewSigner(storage Storage, signKey *bls.SignKey) *Signer {
	verKey,err := signKey.VerKey()
	if err != nil {
		log.Panicf("error getting verkey from sign key: %v", err)
	}
	return &Signer{
		ChainStore: storage,
		SignKey: signKey,
		VerKey: verKey,
		Validators: []ValidatorFunc{
			IsValidOwnerSig,
		},
	}
}

func (n *Signer) NodeId() (string) {
	return common.BytesToAddress(n.VerKey.Bytes()).Hex()
}

func (n *Signer) CanSignBlock(ctx context.Context, block *consensuspb.Block) (bool,error) {
	if block.SignableBlock == nil {
		return false, nil
	}

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

	return true, nil
}

func (n *Signer) SignBlock(ctx context.Context, block *consensuspb.Block) (*consensuspb.Block, error) {
	if block.SignableBlock == nil {
		return nil, fmt.Errorf("no signable block")
	}

	hsh,err := consensus.BlockToHash(block)
	if err != nil {
		return nil, fmt.Errorf("error hashing block: %v", err)
	}

	sigBytes,err := n.SignKey.Sign(hsh.Bytes())
	if err != nil {
		return nil, fmt.Errorf("error signing: %v", err)
	}

	sig := &consensuspb.Signature{
		Creator: n.NodeId(),
		Signature: sigBytes,
		Type: consensuspb.BLSGroupSig,
	}

	block.Signatures = append(block.Signatures, sig)

	return block,nil
}
