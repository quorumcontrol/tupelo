package notary

import (
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"context"
	"github.com/ethereum/go-ethereum/log"
	"github.com/gogo/protobuf/proto"
	"fmt"
	"reflect"
)

var DefaultTransactorRegistry TransactorRegistry

type TransactorState struct {
	MutatableTip *consensuspb.ChainTip
	Signer *Signer
	History History
	// A block can be mutated (by adding individual transaction signatures)
	MutatableBlock *consensuspb.Block
	Transaction interface{}
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
	transaction := (state.Transaction).(*consensuspb.Transaction)

	log.Debug("processing transaction", "id", transaction.Id)
	transactorRegistryEntry,ok := tr[transaction.Type]
	if ok {
		instanceType := proto.MessageType(transactorRegistryEntry.Unmarshaler)
		// instanceType will be a pointer type, so call Elem() to get the original Type and then interface
		// so that we can change it to the kind of object we want
		instance := reflect.New(instanceType.Elem()).Interface()
		err := proto.Unmarshal(transaction.Payload, instance.(proto.Message))
		if err != nil {
			log.Debug("error unmarshaling", "error", err)
			return nil, true, fmt.Errorf("error unmarshaling: %v", err)
		}
		log.Trace("executing transactor with instance", "instance", instance)
		state.Transaction = instance
		return transactorRegistryEntry.Transactor(ctx, state)
	}

	log.Debug("unknown transaction type: ", "type", transaction.Type)
	return state, false, nil
}

func UpdateOwnershipTransactor (ctx context.Context, state *TransactorState) (mutatedState *TransactorState, shouldInterrupt bool, err error) {
	castTransaction := (state.Transaction).(*consensuspb.UpdateOwnershipTransaction)
	state.MutatableTip.Authorizations = castTransaction.Authorizations
	state.MutatableTip.Authentication = castTransaction.Authentication

	return state, false, nil
}
//
//func SendCoinTransactor(ctx context.Context, chain *consensuspb.Chain, block *consensuspb.Block, transaction interface{}) (mutatedChain *consensuspb.Chain, shouldInterrupt bool, err error) {
//	castTransaction := (transaction).(*consensuspb.SendCoinTransaction)
//
//
//
//}