package messages

import (
	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/differencedigest/ibf"
	"github.com/quorumcontrol/tupelo/gossip3/types"
)

type MetadataMap map[string]interface{}
type SignerMap map[string]*types.Signer

type Remove struct {
	Key []byte
}

type GetStrata struct{}

type ValidatorClear struct{}
type ValidatorWorking struct{}
type SubscribeValidatorWorking struct {
	Actor *actor.PID `msg:"-"`
}

type Subscribe struct {
	Subscriber *actor.PID
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
type DoOneGossip struct {
	Why string
}

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

type GetThreadsafeReader struct{}

type NewValidCurrentState struct {
	CurrentState *CurrentState
	Key          []byte
	Value        []byte
}

type SignatureWrapper struct {
	Internal         bool
	ConflictSetID    string
	RewardsCommittee []*types.Signer
	Signers          SignerMap
	Signature        *Signature
	Metadata         MetadataMap
}

type SignatureVerification struct {
	Verified  bool
	Message   []byte
	Signature []byte
	VerKeys   [][]byte
}

type TransactionWrapper struct {
	ConflictSetID string
	TransactionID []byte
	Transaction   *Transaction
	Accepted      bool
	Key           []byte
	Value         []byte
	Metadata      MetadataMap
}

type MemPoolCleanup struct {
	Transactions [][]byte
}

type BulkRemove struct {
	ObjectIDs [][]byte
}

type SendingDone struct{}
