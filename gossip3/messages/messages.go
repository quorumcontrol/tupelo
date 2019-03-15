package messages

import (
	"context"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/opentracing/opentracing-go"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/types"
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
}

type CurrentStateWrapper struct {
	Internal            bool
	Verified            bool
	CurrentState        *extmsgs.CurrentState
	Metadata            MetadataMap
	Key                 []byte
	Value               []byte
	CleanupTransactions []*TransactionWrapper
}

type TransactionWrapper struct {
	ConflictSetID string
	TransactionID []byte
	Transaction   *extmsgs.Transaction
	PreFlight     bool
	Accepted      bool
	Stale         bool
	Key           []byte
	Value         []byte
	Metadata      MetadataMap
	Context       context.Context
}

type contextSpanKey struct{}

var parentSpanKey = contextSpanKey{}

// StartTrace starts the parent trace of a transactionwrapper
func (tw *TransactionWrapper) StartTrace() opentracing.Span {
	parent, ctx := opentracing.StartSpanFromContext(context.Background(), "transaction")
	ctx = context.WithValue(ctx, parentSpanKey, parent)
	tw.Context = ctx
	return parent
}

// StartTrace starts the parent trace of a transactionwrapper
func (tw *TransactionWrapper) StopTrace() {
	val := tw.Context.Value(parentSpanKey)
	val.(opentracing.Span).Finish()
}

func (tw *TransactionWrapper) NewSpan(name string) opentracing.Span {
	sp, ctx := opentracing.StartSpanFromContext(tw.Context, name)
	tw.Context = ctx
	return sp
}

func (tw *TransactionWrapper) LogKV(key string, value interface{}) {
	sp := opentracing.SpanFromContext(tw.Context)
	sp.LogKV(key, value)
}

type MemPoolCleanup struct {
	Transactions [][]byte
}

type BulkRemove struct {
	ObjectIDs [][]byte
}

type SendingDone struct{}

type ActivateSnoozingConflictSets struct {
	ObjectID []byte
}

type ValidateTransaction struct {
	Key   []byte
	Value []byte
}
