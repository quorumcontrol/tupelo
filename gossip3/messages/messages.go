//go:generate msgp

package messages

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/differencedigest/ibf"
)

type Store struct {
	Key   []byte
	Value []byte
}

type Remove struct {
	Key []byte
}

type GetStrata struct{}

type GetSyncer struct {
	Kind string
}

type SyncDone struct{}

type ValidatorClear struct{}
type ValidatorWorking struct{}
type SubscribeValidatorWorking struct {
	Actor *actor.PID `msg:"-"`
}

type GetPrefix struct {
	Prefix []byte
}

type Get struct {
	Key []byte
}

type system interface {
	GetRandomSyncer() *actor.PID
}
type StartGossip struct {
	System system `msg:"-"`
}
type DoOneGossip struct{}

type GetIBF struct {
	Size int
}

type DoPush struct {
	System system `msg:"-"`
}

type ProvideStrata struct {
	Strata    ibf.DifferenceStrata `msg:"-"`
	Validator *actor.PID           `msg:"-"`
}

type ProvideBloomFilter struct {
	Filter    ibf.InvertibleBloomFilter `msg:"-"`
	Validator *actor.PID                `msg:"-"`
}

type RequestIBF struct {
	Count  int
	Result *ibf.DecodeResults `msg:"-"`
}

type RequestKeys struct {
	Keys []uint64
}

type SendPrefix struct {
	Prefix      []byte
	Destination *actor.PID `msg:"-"`
}

type RoundTransition struct {
	NextRound uint64
}

type Transaction struct {
	ObjectID    []byte
	PreviousTip []byte
	NewTip      []byte
	Payload     []byte
}

type Signature struct {
	TransactionID []byte
	ObjectID      []byte
	Tip           []byte
	Signers       []bool
	Signature     []byte
	ConflictSetID string
	Internal      bool `msg:"-"`
}

type NewValidatedTransaction struct {
	ConflictSetID string
	ObjectID      []byte
	TransactionID []byte
	OldTip        []byte
	NewTip        []byte
}

type GetTip struct {
	ObjectID []byte
}

type CurrentState struct {
	ObjectID  []byte
	Tip       []byte
	OldTip    []byte // This is a big deal, worth talking through
	Signature Signature
}

func (cs *CurrentState) StorageKey() []byte {
	return append(cs.ObjectID, cs.OldTip...)
}

func (cs *CurrentState) CurrentKey() []byte {
	return append(cs.ObjectID)
}

func (cs *CurrentState) MustBytes() []byte {
	bits, err := cs.MarshalMsg(nil)
	if err != nil {
		panic(fmt.Errorf("error marshaling current state: %v", err))
	}
	return bits
}

type NewValidCurrentState struct {
	CurrentState *CurrentState
	Key          []byte
	Value        []byte
}

type MemPoolCleanup struct {
	Transactions [][]byte
}

type BulkRemove struct {
	ObjectIDs [][]byte
}

type SendingDone struct{}
