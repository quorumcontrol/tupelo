package signer

import (
	"fmt"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/typecaster"
	"strings"
)

func init() {
	typecaster.AddType(setDataPayload{})
	cbornode.RegisterCborType(setDataPayload{})
}

type setDataPayload struct {
	Path  string
	Value interface{}
}

func setData(tree *dag.BidirectionalTree, transaction *chaintree.Transaction) (valid bool, codedErr chaintree.CodedError) {
	payload := &setDataPayload{}
	err := typecaster.ToType(transaction.Payload, payload)
	if err != nil {
		return false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error casting payload: %v", err)}
	}

	err = tree.Set(strings.Split(payload.Path, "/"), payload.Value)
	if err != nil {
		return false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error setting: %v", err)}
	}

	return true, nil
}
