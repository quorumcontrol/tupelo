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
	Signers       map[string]bool
	Signature     []byte
}

const (
	ProtocolTypeWants int = iota
	ProtocolTypeError
	ProtocolTypeIBF
	ProtocolTypeStrata
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
}

type ConflictSetQuery struct {
	Key []byte
}

type ConflictSetQueryResponse struct {
	Key  []byte
	Done bool
}

type msgPackObject interface {
	MarshalMsg([]byte) ([]byte, error)
	UnmarshalMsg([]byte) ([]byte, error)
}

func toProtocolMessage(msg msgPackObject) (ProtocolMessage, error) {

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
		pm.MessageType = ProtocolTypeIBF
	default:
		return ProtocolMessage{}, fmt.Errorf("unknown type: %v", t)
	}

	return pm, nil
}

func fromProtocolMessage(pm *ProtocolMessage) (msgPackObject, error) {
	var out msgPackObject
	switch pm.MessageType {
	case ProtocolTypeWants:
		out = &WantMessage{}
	case ProtocolTypeIBF:
		out = &ibf.InvertibleBloomFilter{}
	case ProtocolTypeStrata:
		out = &ibf.DifferenceStrata{}
	default:
		return nil, fmt.Errorf("unknown type code: %v", pm.MessageType)
	}
	_, err := out.UnmarshalMsg(pm.Payload)
	return out, err
}
