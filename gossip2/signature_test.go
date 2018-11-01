package gossip2

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTransactionIDFromSignatureKey(t *testing.T) {
	transaction := Transaction{
		ObjectID:    []byte("himynameisalongobjectidthatwillhavemorethan64bits"),
		PreviousTip: []byte(""),
		NewTip:      []byte("zdpuAs5LQAGsXbGTF3DbfGVkRw4sWJd4MzbbigtJ4zE6NNJrr"),
		Payload:     []byte("thisisthepayload"),
	}
	signature := Signature{
		TransactionID: transaction.ID(),
	}

	assert.Equal(t, transaction.StoredID(), transactionIDFromSignatureKey(signature.StoredID(transaction.ToConflictSet().ID())))
}
