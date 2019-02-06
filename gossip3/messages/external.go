//go:generate msgp

package messages

import (
	"encoding/binary"
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/differencedigest/ibf"
)

func init() {
	RegisterEncodable(Ping{})
	RegisterEncodable(Pong{})
	RegisterEncodable(Store{})
	RegisterEncodable(GetSyncer{})
	RegisterEncodable(SyncDone{})
	RegisterEncodable(NoSyncersAvailable{})
	RegisterEncodable(SyncerAvailable{})
	RegisterEncodable(CurrentState{})
	RegisterEncodable(Signature{})
	RegisterEncodable(Transaction{})
	RegisterEncodable(GetTip{})
	RegisterEncodable(ActorPID{})
	RegisterEncodable(ProvideStrata{})
	RegisterEncodable(ProvideBloomFilter{})
	RegisterEncodable(RequestKeys{})
	RegisterEncodable(RequestIBF{})
	RegisterEncodable(TipSubscription{})
}

type DestinationHolder struct {
	Destination *ActorPID
}

func (dh *DestinationHolder) SetDestination(newDestination *ActorPID) {
	dh.Destination = newDestination
}

func (dh *DestinationHolder) GetDestination() *ActorPID {
	return dh.Destination
}

type DestinationSettable interface {
	SetDestination(*ActorPID)
	GetDestination() *ActorPID
}

type Ping struct {
	Msg string
}

type Pong struct {
	Msg string
}

type Store struct {
	Key        []byte
	Value      []byte
	SkipNotify bool `msg:"-"`
}

type GetSyncer struct {
	Kind string
}

type RequestIBF struct {
	Count int
}

type SyncDone struct{}

type NoSyncersAvailable struct{}

type SyncerAvailable struct {
	DestinationHolder
}

type TipSubscription struct {
	Unsubscribe bool
	ObjectID    []byte
}

type CurrentState struct {
	Signature *Signature
}

func (cs *CurrentState) CommittedKey() []byte {
	return []byte(cs.Signature.ConflictSetID())
}

func (cs *CurrentState) CurrentKey() []byte {
	return append(cs.Signature.ObjectID)
}

func (cs *CurrentState) MustBytes() []byte {
	bits, err := cs.MarshalMsg(nil)
	if err != nil {
		panic(fmt.Errorf("error marshaling current state: %v", err))
	}
	return bits
}

type GetTip struct {
	ObjectID []byte
}

type RequestKeys struct {
	Keys []uint64
}

type Signature struct {
	TransactionID []byte
	ObjectID      []byte
	PreviousTip   []byte
	NewTip        []byte
	View          uint64
	Cycle         uint64
	Signers       []byte // this is a marshaled BitArray from github.com/Workiva/go-datastructures
	Signature     []byte
}

func (sig *Signature) GetSignable() []byte {
	return append(append(sig.ObjectID, append(sig.PreviousTip, sig.NewTip...)...), append(uint64ToBytes(sig.View), uint64ToBytes(sig.Cycle)...)...)
}

func (sig *Signature) ConflictSetID() string {
	return ConflictSetID(sig.ObjectID, sig.PreviousTip)
}

func uint64ToBytes(id uint64) []byte {
	a := make([]byte, 8)
	binary.BigEndian.PutUint64(a, id)
	return a
}

type Transaction struct {
	ObjectID    []byte
	PreviousTip []byte
	NewTip      []byte
	Payload     []byte
	State       [][]byte
}

func (t *Transaction) ConflictSetID() string {
	return ConflictSetID(t.ObjectID, t.PreviousTip)
}

func ConflictSetID(objectID, previousTip []byte) string {
	return string(crypto.Keccak256(append(objectID, previousTip...)))
}

type ProvideStrata struct {
	DestinationHolder

	Strata *ibf.DifferenceStrata
}

type ProvideBloomFilter struct {
	DestinationHolder

	Filter *ibf.InvertibleBloomFilter
}

type ActorPID struct {
	Address string
	Id      string
}

func ToActorPid(a *actor.PID) *ActorPID {
	if a == nil {
		return nil
	}
	return &ActorPID{
		Address: a.Address,
		Id:      a.Id,
	}
}

func FromActorPid(a *ActorPID) *actor.PID {
	return actor.NewPID(a.Address, a.Id)
}
