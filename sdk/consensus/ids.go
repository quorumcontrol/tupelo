package consensus

import (
	"encoding/binary"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/messages/v2/build/go/signatures"
)

func RequestID(req *services.AddBlockRequest) []byte {
	//TODO: Right now the Payload includes a "state" section which is a number of
	// state blocks. Since those are arbitrary, you can change your transactionID
	// without altering the state transition.

	return crypto.Keccak256(appendMultiple(
		req.ObjectId,
		req.PreviousTip,
		uint64ToBytes(req.Height),
		req.NewTip,
		req.Payload,
	))
}

func ConflictSetID(objectID []byte, height uint64) string {
	return string(crypto.Keccak256(append(objectID, uint64ToBytes(height)...)))
}

func GetSignable(sig *signatures.TreeState) []byte {
	return appendMultiple(
		sig.ObjectId,
		sig.PreviousTip,
		sig.NewTip,
		uint64ToBytes(sig.View),
		uint64ToBytes(sig.Cycle),
	)
}

func uint64ToBytes(id uint64) []byte {
	a := make([]byte, 8)
	binary.BigEndian.PutUint64(a, id)
	return a
}

func appendMultiple(bits ...[]byte) []byte {
	var retSlice []byte
	for _, b := range bits {
		retSlice = append(retSlice, b...)
	}
	return retSlice
}
