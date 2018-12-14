//go:generate msgp

package messages

func init() {
	RegisterEncodable(Ping{})
	RegisterEncodable(Pong{})
}

type Ping struct {
	Msg string
}

type Pong struct {
	Msg string
}

type Store struct {
	Key   []byte
	Value []byte
}

type GetSyncer struct {
	Kind string
}

type SyncDone struct{}

type CurrentState struct {
	ObjectID  []byte
	Tip       []byte
	OldTip    []byte // This is a big deal, worth talking through
	Signature Signature
}

type Signature struct {
	TransactionID []byte
	ObjectID      []byte
	Tip           []byte
	Signers       []bool
	Signature     []byte
	ConflictSetID string
	Internal      bool `msg:"-"`
}

type Transaction struct {
	ObjectID    []byte
	PreviousTip []byte
	NewTip      []byte
	Payload     []byte
}
