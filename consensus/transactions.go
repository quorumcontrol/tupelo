package consensus

import (
	"fmt"
	"strings"

	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/typecaster"
)

func init() {
	typecaster.AddType(setDataPayload{})
	typecaster.AddType(setOwnershipPayload{})
	cbornode.RegisterCborType(setDataPayload{})
	cbornode.RegisterCborType(setOwnershipPayload{})
}

type setDataPayload struct {
	Path  string
	Value interface{}
}

func SetDataTransaction(tree *dag.BidirectionalTree, transaction *chaintree.Transaction) (valid bool, codedErr chaintree.CodedError) {
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

type setOwnershipPayload struct {
	Authentication []*PublicKey
}

func SetOwnershipTransaction(tree *dag.BidirectionalTree, transaction *chaintree.Transaction) (valid bool, codedErr chaintree.CodedError) {
	payload := &setOwnershipPayload{}
	err := typecaster.ToType(transaction.Payload, payload)
	if err != nil {
		return false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error casting payload: %v", err)}
	}

	authenticationPath := []string{"_qc", "authentications"}

	err = tree.Set(authenticationPath, payload.Authentication)
	if err != nil {
		return false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error setting: %v", err)}
	}

	return true, nil
}
