package notary

import (
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"context"
	"github.com/ethereum/go-ethereum/log"
	"github.com/gogo/protobuf/proto"
	"fmt"
	"reflect"
	"github.com/quorumcontrol/qc3/consensus"
)

var DefaultTransactorRegistry TransactorRegistry

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
		Unmarshaler: "consensuspb.UpdateOwnershipTransaction",
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
	log.Debug("processing transaction", "id", state.Transaction.Id)
	transactorRegistryEntry,ok := tr[state.Transaction.Type]
	if ok {
		log.Trace("executing transactor", "id", state.Transaction.Id)
		typed,err := tr.typedTransactionFrom(state.Transaction)
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

func SendCoinTransaction (ctx context.Context, state *TransactorState) (mutatedState *TransactorState, shouldInterrupt bool, err error) {
	//transaction := (state.TypedTransaction).(*consensuspb.SendCoinTransaction)

	// go back in time to the last validated balance transaction or the genesis block, then play up to this transaction
	history := state.History
	history.StoreBlocks([]*consensuspb.Block{state.MutatableBlock})

	iterator := history.IteratorFrom(state.MutatableBlock, state.Transaction)
	// start by just looking at genesis
	for iterator != nil {

	}


	return state, false, nil
}

func (tr TransactorRegistry) typedTransactionFrom(transaction *consensuspb.Transaction) (interface{},error) {
	transactorRegistryEntry,ok := tr[transaction.Type]
	if ok {
		instanceType := proto.MessageType(transactorRegistryEntry.Unmarshaler)
		// instanceType will be a pointer type, so call Elem() to get the original Type and then interface
		// so that we can change it to the kind of object we want
		instance := reflect.New(instanceType.Elem()).Interface()
		err := proto.Unmarshal(transaction.Payload, instance.(proto.Message))
		if err != nil {
			return nil, fmt.Errorf("error unmarshaling: %v", err)
		}
		return instance, nil
	}
	return nil, nil
}
