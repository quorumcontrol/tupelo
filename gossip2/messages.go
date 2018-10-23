//go:generate msgp
package gossip2

import (
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
	Signers       [][]byte
	Signature     []byte
}

type WantMessage struct {
	Keys []uint64
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
