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
	"bytes"
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
	DefaultTransactorRegistry[consensuspb.RECEIVE_COIN] = &TransactorRegistryEntry{
		Transactor:  ReceiveCoinTransactor,
		Unmarshaler: receiveCoinTransaction,
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

	iterator,err := iteratorAtLastBalanceOrGenerator(transactionInQuestion.Name, state.MutatableBlock, state.Transaction, history)
	if err != nil {
		return state, true, fmt.Errorf("error getting iterator: %v", err)
	}

	balance,_,valid,err := balanceAndSpentTransactionsSince(iterator, transactionInQuestion.Name, state)
	if err != nil {
		return state,true, fmt.Errorf("error iterating over transactions: %v", err)
	}
	if !valid {
		return state, true, nil
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

func ReceiveCoinTransactor(ctx context.Context, state *TransactorState) (mutatedState *TransactorState, shouldInterrupt bool, err error) {
	transactionInQuestion := (state.TypedTransaction).(*consensuspb.ReceiveCoinTransaction)
	_typedSend,err := typedTransactionFrom(transactionInQuestion.SendTransaction, sendCoinTransaction)
	if err != nil {
		return state, true, fmt.Errorf("error unmarshaling send coin transaction: %v", err)
	}
	sendInQuestion := _typedSend.(*consensuspb.SendCoinTransaction)
	// assert that the transaction is signed by the group
	if transactionInQuestion.Signature == nil {
		log.Error("no signature for receive blocK", "transactionId", state.Transaction.Id)
		return state, true, nil
	}

	if !bytes.Equal(transactionInQuestion.Signature.Memo, []byte("tx:" +transactionInQuestion.SendTransaction.Id)) {
		log.Error("memo didn't match", "transactionId", state.Transaction.Id, "memo", string(transactionInQuestion.Signature.Memo), "expectedMemo", []byte("tx:" + transactionInQuestion.SendTransaction.Id))
		return state, true, nil
	}

	if transactionInQuestion.Signature.Creator != state.Signer.Group.Id {
		log.Error("creator wasn't right", "transactionId", state.Transaction.Id, "creator", transactionInQuestion.Signature.Creator, "expectedCreator", state.Signer.Group.Id)
		return state, true, nil
	}

	hsh,err := consensus.TransactionToHash(transactionInQuestion.SendTransaction)
	if err != nil {
		return state, true, fmt.Errorf("error hashing block: %v",err)
	}

	isVerified,err := state.Signer.Group.VerifySignature(hsh.Bytes(),transactionInQuestion.Signature)
	if err != nil {
		return state, true, fmt.Errorf("error hashing block: %v",err)
	}

	if !isVerified {
		return state, true, nil
	}

	// go back in time to the last validated balance transaction or the genesis block, then play up to this transaction
	history := state.History
	history.StoreBlocks([]*consensuspb.Block{state.MutatableBlock})

	iterator,err := iteratorAtLastBalanceOrGenerator(sendInQuestion.Name, state.MutatableBlock, state.Transaction, history)
	if err != nil {
		return state, true, fmt.Errorf("error getting iterator: %v", err)
	}

	_,spentTransactions,valid, err := balanceAndSpentTransactionsSince(iterator, sendInQuestion.Name, state)
	if err != nil {
		return state,true, fmt.Errorf("error iterating over transactions: %v", err)
	}
	if !valid {
		return state, true, nil
	}

	log.Debug("spent transactions", "spentTransactions", spentTransactions)
	// now we have a balance, we can see if it's greater than the current spend
	if !hasId(spentTransactions, sendInQuestion.Id) {
		log.Debug("send id is not spent", "transactionId", state.Transaction.Id)
		// if the balance is bigger, we can sign this transaction
		blck,err := state.Signer.SignTransaction(ctx, state.MutatableBlock, state.Transaction)
		if err != nil {
			return state, true, fmt.Errorf("error signing transaction: %v", err)
		}
		state.MutatableBlock = blck
		return state, false, nil
	}
	log.Info("attempted double spend","id", state.Transaction.Id, "sendTransactionId", sendInQuestion.Id, "receiving", sendInQuestion.Amount)
	// if balance isn't bigger, we should interrupt the signing of this block
	return state, true, nil
}


func iteratorAtLastBalanceOrGenerator(coinName string, currentBlock *consensuspb.Block, transaction *consensuspb.Transaction, history consensus.History) (consensus.TransactionIterator, error) {
	var lastSeenTransaction *consensuspb.Transaction

	iterator := history.IteratorFrom(currentBlock, transaction)

	// get most recent balance transaction and/or go all the way back to genesis
	for iterator.Prev() != nil {
		lastSeenTransaction = iterator.Transaction()

		if lastSeenTransaction.Type == consensuspb.BALANCE {
			typed,err := typedTransactionFrom(lastSeenTransaction, balanceTransaction)
			if err != nil {
				return nil, fmt.Errorf("error getting typed transaction: %v", err)
			}
			typedLastBalance := (typed).(*consensuspb.BalanceTransaction)

			if typedLastBalance.Name == coinName {
				break
			}
		}
		iterator = iterator.Prev()
	}

	return iterator, nil
}

func balanceAndSpentTransactionsSince(iterator consensus.TransactionIterator, coinName string, state *TransactorState) (balance uint64, spentTransactions []string, valid bool, err error) {
	// if there is a balance transaction, see if it's been signed
	if iterator.Transaction().Type == consensuspb.BALANCE {
		// is the transaction signed?
		isSigned,err := isTransactionAppoved(state.Signer, iterator.Block(), iterator.Transaction())
		if err != nil {
			return 0, nil, false, fmt.Errorf("error getting is signed: %v", err)
		}
		if !isSigned {
			return 0, nil, false, nil
		}

		typed,err := typedTransactionFrom(iterator.Transaction(), balanceTransaction)
		if err != nil {
			return 0,nil, false, fmt.Errorf("error getting typed transaction: %v", err)
		}
		typedLastBalance := (typed).(*consensuspb.BalanceTransaction)

		// if we got here, then we have a signed balance transaction
		balance = typedLastBalance.Balance
		spentTransactions = typedLastBalance.Transactions
	} else {
		// if there is not a last balance transaction, then the last block in history *must* be a genesis block
		if iterator.Block().SignableBlock.Sequence != 0 {
			return 0,nil, false, nil
		}
	}
	// get most recent balance transaction and/or go all the way back to genesis
	for iterator != nil && iterator.Transaction() != state.Transaction {
		log.Trace("next iterator")
		transaction := iterator.Transaction()
		log.Debug("processing transaction", "transactionId", transaction.Id, "type", transaction.Type)
		if transaction.Type == consensuspb.RECEIVE_COIN {
			isSigned,err := isTransactionAppoved(state.Signer, iterator.Block(), transaction)
			if !isSigned {
				return 0, nil, false, fmt.Errorf("receive coin was unsigned: %v", err)
			}
			_typedReceived,err := typedTransactionFrom(transaction, receiveCoinTransaction)
			if err != nil {
				return 0, nil,false, fmt.Errorf("error unmarshaling receive coin transaction: %v", err)
			}
			typedReceived := _typedReceived.(*consensuspb.ReceiveCoinTransaction)

			_typedSend,err := typedTransactionFrom(typedReceived.SendTransaction, sendCoinTransaction)
			if err != nil {
				return 0, nil, false, fmt.Errorf("error unmarshaling send coin transaction: %v", err)
			}
			typedSend := _typedSend.(*consensuspb.SendCoinTransaction)

			spentTransactions = append(spentTransactions, typedSend.Id)
			if typedSend.Name == coinName {
				balance += typedSend.Amount
			}
		}
		if transaction.Type == consensuspb.MINT_COIN {
			isSigned,err := isTransactionAppoved(state.Signer, iterator.Block(), transaction)
			if !isSigned {
				return 0, nil, false, nil
			}
			log.Debug("mint transaction is approved")
			_typed,err := typedTransactionFrom(transaction, mintCoinTransaction)
			if err != nil {
				return 0, nil, false, fmt.Errorf("error unmarshaling receive coin transaction: %v", err)
			}
			typed := _typed.(*consensuspb.MintCoinTransaction)

			if typed.Name == coinName {
				log.Debug("adding amount to balance", "amount", typed.Amount)
				balance += typed.Amount
			}
		}
		if transaction.Type == consensuspb.SEND_COIN {
			// we don't even check to see if a send coin is signed, because it's a negative balance so
			// the user isn't incentivized to cheat here

			_typed,err := typedTransactionFrom(transaction, sendCoinTransaction)
			if err != nil {
				return 0,nil, false, fmt.Errorf("error unmarshaling receive coin transaction: %v", err)
			}
			typed := _typed.(*consensuspb.SendCoinTransaction)

			if typed.Name == coinName {
				balance -= typed.Amount
			}
		}
		iterator = iterator.Next()
		log.Debug("next iterator", "transactionId", iterator.Transaction().Id, "type", iterator.Transaction().Type)
	}

	return balance,spentTransactions, true,nil
}

func hasId(ids []string, id string) bool {
	for _,str := range ids {
		if str == id {
			return true
		}
	}
	return false
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
