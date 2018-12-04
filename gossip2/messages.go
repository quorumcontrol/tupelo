//go:generate msgp
package gossip2

import (
	"fmt"

	"github.com/quorumcontrol/differencedigest/ibf"
)

type ConflictSet struct {
	ObjectID []byte
	Tip      []byte
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
}

const (
	ProtocolTypeWants int = iota
	ProtocolTypeError
	ProtocolTypeIBF
	ProtocolTypeStrata
	ProtocolTypeCurrentState
)

type ProtocolMessage struct {
	Code        int
	Error       string
	MessageType int
	Payload     []byte
}

type WantMessage struct {
	Keys []uint64
}

type CurrentState struct {
	ObjectID  []byte
	Tip       []byte
	Signature Signature
}

func WantMessageFromDiff(objs []ibf.ObjectId) *WantMessage {
	ints := make([]uint64, len(objs))
	for i, objId := range objs {
		ints[i] = uint64(objId)
	}
	return &WantMessage{Keys: ints}
}

type ProvideMessage struct {
	Key   []byte
	Value []byte
	Last  bool
	From  string
}

type ConflictSetQuery struct {
	Key []byte
}

type ConflictSetQueryResponse struct {
	Key  []byte
	Done bool
}

type TipQuery struct {
	ObjectID []byte
}

type ChainTreeSubscriptionRequest struct {
	ObjectID []byte
}

type msgPackObject interface {
	MarshalMsg([]byte) ([]byte, error)
	UnmarshalMsg([]byte) ([]byte, error)
}

func ToProtocolMessage(msg msgPackObject) (ProtocolMessage, error) {

	oBytes, err := msg.MarshalMsg(nil)
	if err != nil {
		return ProtocolMessage{}, fmt.Errorf("error marshaling message: %v", err)
	}
	pm := ProtocolMessage{
		Payload: oBytes,
	}

	switch t := msg.(type) {
	case *WantMessage:
		pm.MessageType = ProtocolTypeWants
	case *ibf.InvertibleBloomFilter:
		pm.MessageType = ProtocolTypeIBF
	case *ibf.DifferenceStrata:
		pm.MessageType = ProtocolTypeStrata
	case *CurrentState:
		pm.MessageType = ProtocolTypeCurrentState
	default:
		return ProtocolMessage{}, fmt.Errorf("unknown type: %v", t)
	}

	return pm, nil
}

func FromProtocolMessage(pm *ProtocolMessage) (msgPackObject, error) {
	var out msgPackObject
	switch pm.MessageType {
	case ProtocolTypeWants:
		out = &WantMessage{}
	case ProtocolTypeIBF:
		out = &ibf.InvertibleBloomFilter{}
	case ProtocolTypeStrata:
		out = &ibf.DifferenceStrata{}
	case ProtocolTypeCurrentState:
		out = &CurrentState{}
	default:
		return nil, fmt.Errorf("unknown type code: %v", pm.MessageType)
	}
	_, err := out.UnmarshalMsg(pm.Payload)
	return out, err
}
