package messages

import (
	"fmt"

	"github.com/gogo/protobuf/proto"
	"github.com/quorumcontrol/messages/build/go/services"
	"github.com/quorumcontrol/messages/build/go/signatures"

	"github.com/AsynkronIT/protoactor-go/actor"
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
	State            *signatures.TreeState
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
	CurrentState *signatures.TreeState
	Metadata     MetadataMap
	NextHeight   uint64
}

func (csw *CurrentStateWrapper) MustMarshal() []byte {
	bits, err := proto.Marshal(csw.CurrentState)
	if err != nil {
		panic(fmt.Errorf("error marshaling (should not happen): %v", err))
	}
	return bits
}

type TransactionWrapper struct {
	tracing.ContextHolder

	ConflictSetID string
	TransactionId []byte
	Transaction   *services.AddBlockRequest
	PreFlight     bool
	Accepted      bool
	Stale         bool
	Metadata      MetadataMap
}

type ActivateSnoozingConflictSets struct {
	ObjectId []byte
}

type ValidateTransaction struct {
	Transaction *services.AddBlockRequest
}

type GetNumConflictSets struct{}

type GzipExport struct {
}

type GzipImport struct {
	Payload []byte
}

type ImportCurrentState struct {
	CurrentState *signatures.CurrentState
}

type DoCurrentStateExchange struct {
	Destination *actor.PID
}
