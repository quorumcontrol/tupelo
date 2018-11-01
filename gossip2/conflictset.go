package gossip2

import "github.com/ethereum/go-ethereum/crypto"

var doneBytes = []byte("done")

func init() {
	if len(doneBytes) != 4 {
		panic("doneBytes must be 4 bytes!")
	}
}

func (c *ConflictSet) ID() []byte {
	return crypto.Keccak256(concatBytesSlice(c.ObjectID, c.Tip))
}

func doneIDFromConflictSetID(conflictSetID []byte) []byte {
	return concatBytesSlice(conflictSetID[0:4], doneBytes, []byte{byte(MessageTypeDone)}, conflictSetID)
}

func (c *ConflictSet) DoneID() []byte {
	return doneIDFromConflictSetID(c.ID())
}

func isDone(storage *BadgerStorage, conflictSetID []byte) (bool, error) {
	doneIDPrefix := doneIDFromConflictSetID(conflictSetID)[0:8]

	// TODO: This currently only checks the first 4 bytes of the conflict set,
	// to make this more robust it should check first 4, if multiple keys are returned
	// then it should decode the transaction from the message, and then check the appropiate key
	conflictSetDoneKeys, err := storage.GetKeysByPrefix(doneIDPrefix)
	if err != nil {
		return false, err
	}
	if len(conflictSetDoneKeys) > 0 {
		return true, nil
	}
	return false, nil
}
