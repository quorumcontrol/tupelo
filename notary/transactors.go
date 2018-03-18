package notary

import (
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"context"
	"github.com/ethereum/go-ethereum/log"
	"github.com/gogo/protobuf/proto"
	"fmt"
	"reflect"
	"github.com/quorumcontrol/qc3/consensus"
	"strings"
)

var DefaultTransactorRegistry TransactorRegistry

const (
	updateOwnershipTransaction = "consensuspb.UpdateOwnershipTransaction"
	sendCoinTransaction        = "consensuspb.SendCoinTransaction"
	receiveCoinTransaction     = "consensuspb.ReceiveCoinTransaction"
	balanceTransaction         = "consensuspb.BalanceTransaction"
	mintCoinTransaction        = "consensuspb.MintCoinTransaction"
)

type TransactorState struct {
	MutatableTip *consensuspb.ChainTip
	Signer *Signer
	History consensus.History
	// A block can be mutated (by adding individual transaction signatures)
	MutatableBlock *consensuspb.Block
	Transaction *consensuspb.Transaction
	TypedTransaction interface{}
}

func init() {
	DefaultTransactorRegistry = NewTransactorRegistry()
	DefaultTransactorRegistry[consensuspb.UPDATE_OWNERSHIP] = &TransactorRegistryEntry{
		Transactor: UpdateOwnershipTransactor,
		Unmarshaler: updateOwnershipTransaction,
	}
	DefaultTransactorRegistry[consensuspb.SEND_COIN] = &TransactorRegistryEntry{
		Transactor: SendCoinTransactor,
		Unmarshaler: sendCoinTransaction,
	}
	DefaultTransactorRegistry[consensuspb.MINT_COIN] = &TransactorRegistryEntry{
		Transactor:  MintCoinTransactor,
		Unmarshaler: mintCoinTransaction,
	}
}

type Transactor func(ctx context.Context, state *TransactorState) (mutatedState *TransactorState, shouldInterrupt bool, err error)

type TransactorRegistryEntry struct {
	Unmarshaler string
	Transactor Transactor
}

type TransactorRegistry map[consensuspb.Transaction_TransactionType]*TransactorRegistryEntry

func NewTransactorRegistry() TransactorRegistry {
	return make(TransactorRegistry)
}

func (tr TransactorRegistry) Distribute(ctx context.Context, state *TransactorState) (mutatedState *TransactorState, shouldInterrupt bool, err error) {
	log.Info("processing transaction", "id", state.Transaction.Id)
	transactorRegistryEntry,ok := tr[state.Transaction.Type]
	if ok {
		log.Trace("executing transactor", "id", state.Transaction.Id)
		typed,err := typedTransactionFrom(state.Transaction, transactorRegistryEntry.Unmarshaler)
		if err != nil {
			return state, true, fmt.Errorf("error getting typed transaction: %v", err)
		}
		state.TypedTransaction = typed
		return transactorRegistryEntry.Transactor(ctx, state)
	}

	log.Debug("unknown transaction type: ", "type", state.Transaction.Type)
	return state, false, nil
}

func UpdateOwnershipTransactor (ctx context.Context, state *TransactorState) (mutatedState *TransactorState, shouldInterrupt bool, err error) {
	transaction := (state.TypedTransaction).(*consensuspb.UpdateOwnershipTransaction)
	state.MutatableTip.Authorizations = transaction.Authorizations
	state.MutatableTip.Authentication = transaction.Authentication

	return state, false, nil
}

// TODO: this needs some really good refactoring
// SendCoinTransactin takes the history, looks back in history to find the last balance block (or all the way back to genesis)
// it then plays forward to make sure there aren't any send/receive since that balance/genesis and makes sure that the current
// send is OK... it then will sign this send transaction (if there is enough balance)
func SendCoinTransactor (ctx context.Context, state *TransactorState) (mutatedState *TransactorState, shouldInterrupt bool, err error) {
	transactionInQuestion := (state.TypedTransaction).(*consensuspb.SendCoinTransaction)

	// go back in time to the last validated balance transaction or the genesis block, then play up to this transaction
	history := state.History
	history.StoreBlocks([]*consensuspb.Block{state.MutatableBlock})

	var lastSeenBlock *consensuspb.Block
	var lastSeenTransaction *consensuspb.Transaction
	var lastBalance *consensuspb.Transaction
	var typedLastBalance *consensuspb.BalanceTransaction
  	balance := uint64(0)

	iterator := history.IteratorFrom(state.MutatableBlock, state.Transaction)
	// get most recent balance transaction and/or go all the way back to genesis
	for iterator != nil {
		lastSeenTransaction = iterator.Transaction()
		lastSeenBlock = iterator.Block()

		if lastSeenTransaction.Type == consensuspb.BALANCE {
			typed,err := typedTransactionFrom(lastSeenTransaction, balanceTransaction)
			if err != nil {
				return state, true, fmt.Errorf("error getting typed transaction: %v", err)
			}
			typedLastBalance = (typed).(*consensuspb.BalanceTransaction)

			if typedLastBalance.Name == transactionInQuestion.Name {
				lastBalance = lastSeenTransaction
				break
			}
		}
		iterator = iterator.Prev()
	}

	log.Debug("lastSeenTransaction", "transactionId", lastSeenTransaction.Id, "type", lastSeenTransaction.Type, "lastBalance", lastBalance)

	// if there is a balance transaction, see if it's been signed
	if lastBalance != nil {
		// is the transaction signed?
		isSigned,err := isTransactionAppoved(state.Signer, iterator.Block(), lastBalance)
		if err != nil {
			return state, true, fmt.Errorf("error getting is signed: %v", err)
		}
		if !isSigned {
			return state, true, nil
		}

		// if we got here, then we have a signed balance transaction
		balance = typedLastBalance.Balance
	} else {
		// if there is not a last balance transaction, then the last block in history *must* be a genesis block
		if lastSeenBlock == nil || lastSeenBlock.SignableBlock.Sequence != 0 {
			return state, true, nil
		}
	}

	// now we have a sent balance (or 0) and we play forward from last seen and
	iterator = history.IteratorFrom(lastSeenBlock, lastSeenTransaction)
	// get most recent balance transaction and/or go all the way back to genesis
	for iterator != nil && iterator.Transaction() != state.Transaction {
		log.Trace("next iterator")
		transaction := iterator.Transaction()
		log.Debug("processing transaction", "transactionId", transaction.Id, "type", transaction.Type)
		if transaction.Type == consensuspb.RECEIVE_COIN {
			isSigned,err := isTransactionAppoved(state.Signer, iterator.Block(), transaction)
			if !isSigned {
				return state, true, nil
			}
			_typed,err := typedTransactionFrom(transaction, receiveCoinTransaction)
			if err != nil {
				return state, true, fmt.Errorf("error unmarshaling receive coin transaction: %v", err)
			}
			typed := _typed.(*consensuspb.ReceiveCoinTransaction)

			if typed.SendTransaction.Name == transactionInQuestion.Name {
				balance += typed.SendTransaction.Amount
			}
		}
		if transaction.Type == consensuspb.MINT_COIN {
			isSigned,err := isTransactionAppoved(state.Signer, iterator.Block(), transaction)
			if !isSigned {
				return state, true, nil
			}
			log.Debug("mint transaction is approved")
			_typed,err := typedTransactionFrom(transaction, mintCoinTransaction)
			if err != nil {
				return state, true, fmt.Errorf("error unmarshaling receive coin transaction: %v", err)
			}
			typed := _typed.(*consensuspb.MintCoinTransaction)

			if typed.Name == transactionInQuestion.Name {
				log.Debug("adding amount to balance", "amount", typed.Amount)
				balance += typed.Amount
			}
		}
		if transaction.Type == consensuspb.SEND_COIN {
			// we don't even check to see if a send coin is signed, because it's a negative balance so
			// the user isn't incentivized to cheat here

			_typed,err := typedTransactionFrom(transaction, sendCoinTransaction)
			if err != nil {
				return state, true, fmt.Errorf("error unmarshaling receive coin transaction: %v", err)
			}
			typed := _typed.(*consensuspb.SendCoinTransaction)

			if typed.Name == transactionInQuestion.Name {
				balance -= typed.Amount
			}
		}
		iterator = iterator.Next()
		log.Debug("next iterator", "transactionId", iterator.Transaction().Id, "type", iterator.Transaction().Type)
	}

	log.Debug("calculated balance", "balance", balance, "sending", transactionInQuestion.Amount)
	// now we have a balance, we can see if it's greater than the current spend
	if balance >= transactionInQuestion.Amount {
		// if the balance is bigger, we can sign this transaction
		blck,err := state.Signer.SignTransaction(ctx, state.MutatableBlock, state.Transaction)
		if err != nil {
			return state, true, fmt.Errorf("error signing transaction: %v", err)
		}
		state.MutatableBlock = blck
		return state, false, nil
	}
	log.Info("attempted overspend","id", state.Transaction.Id, "balance", balance, "sending", transactionInQuestion.Amount)
	// if balance isn't bigger, we should interrupt the signing of this block
	return state, true, nil
}

// we allow a chain to mint its own coin, but no one elses
func MintCoinTransactor(ctx context.Context, state *TransactorState) (mutatedState *TransactorState, shouldInterrupt bool, err error) {
	transactionInQuestion := (state.TypedTransaction).(*consensuspb.MintCoinTransaction)
	if strings.HasPrefix(transactionInQuestion.Name, state.MutatableTip.Id) {
		blck,err := state.Signer.SignTransaction(ctx, state.MutatableBlock, state.Transaction)
		if err != nil {
			return state, true, fmt.Errorf("error signing transaction: %v", err)
		}
		state.MutatableBlock = blck
		return state, false, nil
	}
	// otherwise, this is an invalid mint
	return state, true, nil
}

// if the block is signed by the group then we can assume the transaction is ok
// otherwise if an individual transaction is signed by the group *or* this individual signer,
// we can also assert it's already been through the process of checking on this node.
func isTransactionAppoved(signer *Signer, block *consensuspb.Block, transaction *consensuspb.Transaction) (bool,error) {
	isSigned,err := signer.Group.IsBlockSigned(block)
	if err != nil {
		return false, fmt.Errorf("error getting isSigned: %v", err)
	}
	if isSigned {
		return true, nil
	} else {
		isSigned,err := signer.IsTransactionSigned(block, transaction)
		if err != nil {
			return false, fmt.Errorf("error getting isSigned: %v", err)
		}
		return isSigned,nil
	}
}

func typedTransactionFrom(transaction *consensuspb.Transaction, messageType string) (interface{},error) {
	instanceType := proto.MessageType(messageType)
	// instanceType will be a pointer type, so call Elem() to get the original Type and then interface
	// so that we can change it to the kind of object we want
	instance := reflect.New(instanceType.Elem()).Interface()
	err := proto.Unmarshal(transaction.Payload, instance.(proto.Message))
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling: %v", err)
	}
	return instance, nil
}
