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

type ChainStorage interface {
	Set(id string, chain *consensuspb.ChainTip) (error)
	Get(id string) (*consensuspb.ChainTip,error)
}

type Signer struct {
	Group      *Group
	ChainStore ChainStorage
	VerKey     *bls.VerKey
	SignKey    *bls.SignKey
	Validators []ValidatorFunc
}

func NewSigner(storage ChainStorage, group *Group, signKey *bls.SignKey) *Signer {
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

func (n *Signer) catchupTip(ctx context.Context, history consensus.History, tip *consensuspb.ChainTip) error {
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

func (n *Signer) ProcessBlock(ctx context.Context, history consensus.History, block *consensuspb.Block) (processed *consensuspb.Block, err error) {
	if block.SignableBlock == nil {
		log.Debug("no signable block")
		return nil, nil
	}

	chainTip,err := n.GetTip(ctx, block.SignableBlock.ChainId)
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
		newTip, success,err := n.RunTransactions(ctx, chainTip, history, block)
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

			log.Debug("saving chain", "chainId", newTip.Id)
			// TODO: we should not set the store until it's been broadcast on the network
			newTip.LastHash = consensus.MustBlockToHash(signedBlock).Bytes()
			n.ChainStore.Set(newTip.Id, newTip)

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

func (n *Signer) RunTransactions(ctx context.Context, currentTip *consensuspb.ChainTip, history consensus.History, block *consensuspb.Block) (*consensuspb.ChainTip, bool, error) {
	currentState := &TransactorState{
		Signer: n,
		MutatableBlock: block,
		History: history,
		MutatableTip: currentTip,
	}

	for _,transaction := range block.SignableBlock.Transactions {
		currentState.Transaction = transaction
		newState,shouldInterrupt,err := DefaultTransactorRegistry.Distribute(ctx, currentState)
		if err != nil {
			return nil, true, fmt.Errorf("error distributing: %v", err)
		}
		if shouldInterrupt {
			return nil, false, nil
		}
		currentState = newState
	}
	return currentState.MutatableTip, true, nil
}

func (n *Signer) SignBlock(ctx context.Context, block *consensuspb.Block) (*consensuspb.Block, error) {
	return consensus.BlsSignBlock(block, n.SignKey)
}

func (n *Signer) SignTransaction(ctx context.Context, block *consensuspb.Block, transaction *consensuspb.Transaction) (*consensuspb.Block, error) {
	return consensus.BlsSignTransaction(block, transaction, n.SignKey)
}


func (n *Signer) GetTip(ctx context.Context, id string) (*consensuspb.ChainTip,error) {
	chainTip,err := n.ChainStore.Get(id)
	if err != nil {
		log.Debug("error getting existing chain")
		return nil, fmt.Errorf("error getting existing chain: %v", err)
	}
	return chainTip,nil
}

// check if we've signed it or if the group has signed it

func (n *Signer) IsTransactionSigned(block *consensuspb.Block, transaction *consensuspb.Transaction) (bool,error) {
	isSigned,err := n.Group.IsTransactionSigned(block, transaction)
	if err != nil {
		return false, fmt.Errorf("error checking for group sig: %v", err)
	}
	if isSigned {
		return true, nil
	}

	if len(block.TransactionSignatures) == 0 {
		return false, nil
	}
	sigs := sigsByMemo(block.TransactionSignatures)
	sig,ok := sigs["tx:" + transaction.Id]
	if !ok {
		return false,nil
	}

	if sig.Creator != n.Id() {
		return false, nil
	}

	hsh,err := consensus.TransactionToHash(transaction)
	if err != nil {
		return false, fmt.Errorf("error hashing block: %v",err)
	}

	return n.VerKey.Verify(sig.Signature, hsh.Bytes())
}
