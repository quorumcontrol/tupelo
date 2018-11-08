package gossip2

import "github.com/ethereum/go-ethereum/crypto"

func (t *Transaction) ToConflictSet() *ConflictSet {
	return &ConflictSet{ObjectID: t.ObjectID, Tip: t.PreviousTip}
}

// ID in storage is 32bitsConflictSetId|32bitsTransactionHash|fullConflictSetID|fulltransactionHash|"-transaction" or transaction before hash
func (t *Transaction) StoredID() []byte {
	conflictSetID := t.ToConflictSet().ID()
	id := t.ID()
	return storedIDForTransactionIDAndConflictSetID(id, conflictSetID)
}

func storedIDForTransactionIDAndConflictSetID(transactionID []byte, conflictSetID []byte) []byte {
	return concatBytesSlice(conflictSetID[0:4], transactionID[0:4], []byte{byte(MessageTypeTransaction)}, transactionID, conflictSetID)
}

func (t *Transaction) ID() []byte {
	encoded, err := t.MarshalMsg(nil)
	if err != nil {
		panic("error marshaling transaction")
	}
	return crypto.Keccak256(encoded)
}
