package gossip2

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestID(t *testing.T) {
	c := ConflictSet{
		ObjectID: []byte("oid"),
		Tip:      []byte("tid"),
	}
	assert.Len(t, c.ID(), 32)
}


func TestConflictSetIDFromSignatureKey(t *testing.T) {
	transaction := Transaction{
		ObjectID:    []byte("himynameisalongobjectidthatwillhavemorethan64bits"),
		PreviousTip: []byte(""),
		NewTip:      []byte("zdpuAs5LQAGsXbGTF3DbfGVkRw4sWJd4MzbbigtJ4zE6NNJrr"),
		Payload:     []byte("thisisthepayload"),
	}
	signature := Signature{
		TransactionID: transaction.ID(),
	}

	assert.Equal(t, transaction.ToConflictSet().ID(), conflictSetIDFromMessageKey(signature.StoredID(transaction.ToConflictSet().ID())))
}
