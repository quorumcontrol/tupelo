package notary

import (
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"fmt"
	"context"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/ethereum/go-ethereum/log"
	"bytes"
)

type History interface {
	StoreBlocks([]*consensuspb.Block) error
	GetBlock(hsh []byte) (*consensuspb.Block)
	NextBlock(hsh []byte) (*consensuspb.Block)
	Length() (uint64)
}

type Storage interface {
	Set(id string, chain *consensuspb.ChainTip) (error)
	Get(id string) (*consensuspb.ChainTip,error)
}

type Signer struct {
	Group *Group
	ChainStore Storage
	VerKey *bls.VerKey
	SignKey *bls.SignKey
	Validators []ValidatorFunc
}

func NewSigner(storage Storage, group *Group, signKey *bls.SignKey) *Signer {
	verKey,err := signKey.VerKey()
	if err != nil {
		log.Crit("error getting verkey from sign key", "error", err)
	}
	return &Signer{
		ChainStore: storage,
		Group: group,
		SignKey: signKey,
		VerKey: verKey,
		Validators: []ValidatorFunc{
			IsSigned,
			IsValidSequenceNumber,
			IsNotGenesisOrIsValidGenesis,
			IsGenesisOrIsSignedByNecessaryOwners,
		},
	}
}

func (n *Signer) Id() (string) {
	return consensus.BlsVerKeyToAddress(n.VerKey.Bytes()).Hex()
}

func (n *Signer) catchupTip(ctx context.Context, history History, tip *consensuspb.ChainTip) error {
	block := history.NextBlock(tip.LastHash)

	for block != nil {
		_,err := n.ProcessBlock(ctx, history, block)
		if err != nil {
			return fmt.Errorf("error processing block: %v", err)
		}

		hsh,err := consensus.BlockToHash(block)
		if err != nil {
			return fmt.Errorf("error getting hash: %v", err)
		}

		block = history.NextBlock(hsh.Bytes())
	}
	return nil
}

func (n *Signer) tipAndHistoryToChain(ctx context.Context, history History, tip *consensuspb.ChainTip) (*consensuspb.Chain, error) {
	chain := &consensuspb.Chain{
		Id: tip.Id,
		Authentication: tip.Authentication,
		Authorizations: tip.Authorizations,
	}

	if history != nil {
		blocks := make([]*consensuspb.Block, history.Length())

		block := history.GetBlock(tip.LastHash)
		for i := history.Length()-1; i >0 && block != nil; i-- {
			isSigned,err := n.Group.IsSignedByGroup(block)
			if err != nil {
				return nil, fmt.Errorf("error getting is signed: %v", err)
			}
			if !isSigned {
				return nil, fmt.Errorf("invalid history, block was unsigned: %v", err)
			}
			blocks[i] = block
			hsh,err := consensus.BlockToHash(block)
			if err != nil {
				return nil, fmt.Errorf("error getting hash: %v", err)
			}
			block = history.GetBlock(hsh.Bytes())
		}
		chain.Blocks = blocks
	}

	return chain,nil
}

func (n *Signer) ProcessBlock(ctx context.Context, history History, block *consensuspb.Block) (processed *consensuspb.Block, err error) {
	if block.SignableBlock == nil {
		log.Debug("no signable block")
		return nil, nil
	}

	chainTip,err := n.ChainStore.Get(block.SignableBlock.ChainId)
	if err != nil {
		log.Debug("error getting existing chain")
		return nil, fmt.Errorf("error getting existing chain: %v", err)
	}

	if !bytes.Equal(chainTip.LastHash, block.SignableBlock.PreviousHash) && history != nil {
		n.catchupTip(ctx, history, chainTip)
	}

	isValid, err := n.ValidateBlockLevel(ctx, chainTip, block)
	if isValid {
		log.Debug("block is valid")
		startChain,err := n.tipAndHistoryToChain(ctx, history, chainTip)
		if err != nil {
			return nil, fmt.Errorf("error converting history to chain: %v", err)
		}
		chain, success,err := n.RunTransactions(ctx, startChain, block)
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
			n.ChainStore.Set(chain.Id, consensus.ChainToTip(chain))

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

func (n *Signer) ValidateBlockLevel(ctx context.Context, chainTip *consensuspb.ChainTip, block *consensuspb.Block) (bool, error) {
	for i, validatorFunc := range n.Validators {
		isValid,err := validatorFunc(ctx, chainTip, block)
		if err != nil {
			log.Debug("error validating: ", "error",err)
			return false, fmt.Errorf("error getting validation: %v: %v", validatorFunc, err)
		}
		if !isValid {
			log.Debug("failed validation", "index", i)
			return false, nil
		}
	}

	return true, nil
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
