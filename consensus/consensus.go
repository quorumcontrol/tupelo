package consensus

import (
	"fmt"
	"strings"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/gogo/protobuf/proto"
	"github.com/google/uuid"
)

func AddrToDid (addr string) string {
	return fmt.Sprintf("did:qc:%s", addr)
}

func DidToAddr(did string) string {
	segs := strings.Split(did, ":")
	return segs[len(segs) - 1]
}

func EncapsulateTransaction(transactionType consensuspb.Transaction_TransactionType, transaction proto.Message) *consensuspb.Transaction {
	encoded,_ := proto.Marshal(transaction)
	return &consensuspb.Transaction{
		Id: uuid.New().String(),
		Type: transactionType,
		Payload: encoded,
	}
}