package messages

import (
	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/differencedigest/ibf"
)

type Remove struct {
	Key []byte
}

type GetStrata struct{}

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

type RequestIBF struct {
	Count  int
	Result *ibf.DecodeResults `msg:"-"`
}

type SendPrefix struct {
	Prefix      []byte
	Destination *actor.PID `msg:"-"`
}

type RoundTransition struct {
	NextRound uint64
}

type NewValidatedTransaction struct {
	ConflictSetID string
	ObjectID      []byte
	TransactionID []byte
	OldTip        []byte
	NewTip        []byte
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
