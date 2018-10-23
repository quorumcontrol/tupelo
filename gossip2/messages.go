//go:generate msgp
package gossip2

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

type ProvideMessage struct {
	Key   []byte
	Value []byte
}
