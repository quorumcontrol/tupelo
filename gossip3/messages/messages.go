package messages

import (
	"github.com/AsynkronIT/protoactor-go/actor"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/types"
	"github.com/quorumcontrol/tupelo-go-client/tracing"
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

type SendPrefix struct {
	Prefix      []byte
	Destination *actor.PID `msg:"-"`
}

type RoundTransition struct {
	NextRound uint64
}

type GetThreadsafeReader struct{}

type NewValidCurrentState struct {
	CurrentState *extmsgs.CurrentState
	Key          []byte
	Value        []byte
}

type SignatureWrapper struct {
	Internal         bool
	ConflictSetID    string
	RewardsCommittee []*types.Signer
	Signers          SignerMap
	Signature        *extmsgs.Signature
	Metadata         MetadataMap
}

type SignatureVerification struct {
	Verified  bool
	Message   []byte
	Signature []byte
	VerKeys   [][]byte
	Memo      interface{}
}

type CurrentStateWrapper struct {
	tracing.ContextHolder

	Internal     bool
	Verified     bool
	CurrentState *extmsgs.CurrentState
	Metadata     MetadataMap
	Key          []byte
	Value        []byte
}

type TransactionWrapper struct {
	tracing.ContextHolder

	ConflictSetID string
	TransactionID []byte
	Transaction   *extmsgs.Transaction
	PreFlight     bool
	Accepted      bool
	Stale         bool
	Metadata      MetadataMap
}

type SendingDone struct{}

type ActivateSnoozingConflictSets struct {
	ObjectID []byte
}

type ValidateTransaction struct {
	Transaction *extmsgs.Transaction
}

type GetNumConflictSets struct{}

type GzipExport struct {
}

type GzipImport struct {
	Payload []byte
}
