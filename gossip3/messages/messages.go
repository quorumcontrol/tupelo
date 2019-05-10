package messages

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	extmsgs "github.com/quorumcontrol/tupelo-go-sdk/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo-go-sdk/tracing"
)

type MetadataMap map[string]interface{}
type SignerMap map[string]*types.Signer

type RoundTransition struct {
	NextRound uint64
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
	NextHeight   uint64

	FailedTransactions []*TransactionWrapper
}

func (csw *CurrentStateWrapper) MustMarshal() []byte {
	bits, err := csw.CurrentState.MarshalMsg(nil)
	if err != nil {
		panic(fmt.Errorf("error marshaling (should not happen): %v", err))
	}
	return bits
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

type ImportCurrentState struct {
	CurrentState *extmsgs.CurrentState
}

type DoCurrentStateExchange struct {
	Destination *actor.PID
}
