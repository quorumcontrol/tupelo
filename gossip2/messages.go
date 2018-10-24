//go:generate msgp
package gossip2

import (
	"github.com/quorumcontrol/differencedigest/ibf"
)

// ID is just hash of object
type ConflictSet struct {
	ObjectID []byte
	Tip      []byte
}

// ID in storage is 32bitsConflictSetId|32bitsTransactionHash|fulltransactionHash|"-transaction" or transaction before hash
// ID on wire is 32bitsConflictSetId|32bitsTransactionHash
type Transaction struct {
	ObjectID    []byte
	PreviousTip []byte
	NewTip      []byte
	Payload     []byte
}

// ID in storage is 32bitsConflictSetId|32bitssignaturehash|transactionid|signaturehash|"-signature" OR signature before transactionid or before signaturehash
// ID on wire is 32bitsConflictSetId|32bitssignaturehash
// a completed signature looks like 32bitsConflictSetId|"done00000000" (padded done message)
type Signature struct {
	TransactionID []byte
	Signers       map[string]bool
	Signature     []byte
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
