package notary

import (
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"fmt"
	"context"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/ethereum/go-ethereum/log"
)

type Storage interface {
	Set(id string, chain *consensuspb.Chain) (error)
	Get(id string) (*consensuspb.Chain,error)
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
		log.Crit("error getting verkey from sign key", "error", err)
	}
	return &Signer{
		ChainStore: storage,
		SignKey: signKey,
		VerKey: verKey,
		Validators: []ValidatorFunc{
			IsSigned,
			IsNotGenesisOrIsValidGenesis,
			IsGenesisOrIsSignedByNecessaryOwners,
		},
	}
}

func (n *Signer) Id() (string) {
	return consensus.BlsVerKeyToAddress(n.VerKey.Bytes()).Hex()
}

func (n *Signer) ProcessBlock(ctx context.Context, block *consensuspb.Block) (processed *consensuspb.Block, err error) {
	isValid, chain, err := n.ValidateBlockLevel(ctx, block)
	if isValid {
		log.Debug("block is valid")
		chain, success,err := n.RunTransactions(ctx, chain, block)
		if err != nil {
			return nil, fmt.Errorf("error running transactions: %v", err)
		}
		if success {
			log.Debug("transactions run")
			signedBlock,err := n.SignBlock(ctx, block)
			if err != nil {
				log.Debug("error signing block", "error", err)
				return nil, fmt.Errorf("error signing block: %v", err)
			}

			// TODO: For now we keep the entire state of the chain, that should change to just the last block
			chain.Blocks = append(chain.Blocks, signedBlock)

			log.Debug("saving chain", "chainId", chain.Id)
			// TODO: we should set the store until it's been broadcast on the network
			n.ChainStore.Set(chain.Id, chain)

			log.Debug("returning block with no error")
			return signedBlock, nil
		} else {
			log.Debug("failed running transactions")
		}
	} else {
		log.Debug("invalid block level")
	}

	log.Debug("returning nil,nil for processing block")
	return nil, nil
}

func (n *Signer) ValidateBlockLevel(ctx context.Context, block *consensuspb.Block) (bool,*consensuspb.Chain, error) {
	if block.SignableBlock == nil {
		log.Debug("no signable block")
		return false, nil, nil
	}

	currentChain,err := n.ChainStore.Get(block.SignableBlock.ChainId)
	if err != nil {
		log.Debug("error getting existing chain")
		return false, nil, fmt.Errorf("error getting existing chain: %v", err)
	}

	ctx = context.WithValue(ctx, "storage", n.ChainStore)

	for i, validatorFunc := range n.Validators {
		isValid,err := validatorFunc(ctx, currentChain, block)
		if err != nil {
			log.Debug("error validating: ", "error",err)
			return false, nil, fmt.Errorf("error getting validation: %v: %v", validatorFunc, err)
		}
		if !isValid {
			log.Debug("failed validation", "index", i)
			return false, nil, nil
		}
	}

	return true, currentChain, nil
}

func (n *Signer) RunTransactions(ctx context.Context, chainState *consensuspb.Chain, block *consensuspb.Block) (*consensuspb.Chain, bool, error) {
	for _,transaction := range block.SignableBlock.Transactions {
		newState,shouldInterrupt,err := DefaultTransactorRegistry.Distribute(ctx, chainState, block, transaction)
		if err != nil {
			return nil, true, fmt.Errorf("error distributing: %v", err)
		}
		if shouldInterrupt {
			return nil, false, nil
		}
		chainState = newState
	}
	return chainState, true, nil
}

func (n *Signer) SignBlock(ctx context.Context, block *consensuspb.Block) (*consensuspb.Block, error) {
	return consensus.BlsSignBlock(block, n.SignKey)
}
