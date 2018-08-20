package consensus

import (
	"fmt"
	"strings"

	cid "github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/typecaster"
)

const (
	TreePathForAuthentications = "_qc/authentications"
	TreePathForStake           = "_qc/stake"
)

func init() {
	typecaster.AddType(SetDataPayload{})
	typecaster.AddType(setOwnershipPayload{})
	typecaster.AddType(StakePayload{})
	cbornode.RegisterCborType(SetDataPayload{})
	cbornode.RegisterCborType(setOwnershipPayload{})
	cbornode.RegisterCborType(StakePayload{})
}

// SetDataPayload is the payload for a SetDataTransaction
// Path / Value
type SetDataPayload struct {
	Path  string
	Value interface{}
}

func complexType(obj interface{}) bool {
	switch obj.(type) {
	// These are the built in type of go (excluding map) plus cid.Cid
	// Use SetAsLink if attempting to set map
	case bool, byte, complex64, complex128, error, float32, float64, int, int8, int16, int32, int64, string, uint, uint16, uint32, uint64, uintptr, cid.Cid, *bool, *byte, *complex64, *complex128, *error, *float32, *float64, *int, *int8, *int16, *int32, *int64, *string, *uint, *uint16, *uint32, *uint64, *uintptr, *cid.Cid, []bool, []byte, []complex64, []complex128, []error, []float32, []float64, []int, []int8, []int16, []int32, []int64, []string, []uint, []uint16, []uint32, []uint64, []uintptr, []cid.Cid, []*bool, []*byte, []*complex64, []*complex128, []*error, []*float32, []*float64, []*int, []*int8, []*int16, []*int32, []*int64, []*string, []*uint, []*uint16, []*uint32, []*uint64, []*uintptr, []*cid.Cid:
		return false
	default:
		return true
	}
}

// SetDataTransaction just sets a path in a tree to arbitrary data. It makes sure no data is being changed in the _qc path.
func SetDataTransaction(tree *dag.Dag, transaction *chaintree.Transaction) (newTree *dag.Dag, valid bool, codedErr chaintree.CodedError) {
	payload := &SetDataPayload{}
	err := typecaster.ToType(transaction.Payload, payload)
	if err != nil {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error casting payload: %v", err)}
	}

	if payload.Path == "_qc" || strings.HasPrefix(payload.Path, "_qc/") {
		return nil, false, &ErrorCode{Code: 999, Memo: "the path prefix _qc is reserved"}
	}

	if complexType(payload.Value) {
		newTree, err = tree.SetAsLink(strings.Split(payload.Path, "/"), payload.Value)
	} else {
		newTree, err = tree.Set(strings.Split(payload.Path, "/"), payload.Value)
	}
	if err != nil {
		return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error setting: %v", err)}
	}

	return newTree, true, nil
}

type setOwnershipPayload struct {
	Authentication []*PublicKey
}

// SetOwnershipTransaction changes the ownership of a tree by adding a public key array to /_qc/authentications
func SetOwnershipTransaction(tree *dag.Dag, transaction *chaintree.Transaction) (newTree *dag.Dag, valid bool, codedErr chaintree.CodedError) {
	payload := &setOwnershipPayload{}
	err := typecaster.ToType(transaction.Payload, payload)
	if err != nil {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error casting payload: %v", err)}
	}

	newTree, err = tree.SetAsLink(strings.Split(TreePathForAuthentications, "/"), payload.Authentication)
	if err != nil {
		return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error setting: %v", err)}
	}

	return newTree, true, nil
}

type StakePayload struct {
	GroupId string
	Amount  uint64
	DstKey  PublicKey
	VerKey  PublicKey
}

// THIS IS A pre-ALPHA TRANSACTION AND NO RULES ARE ENFORCED! Anyone can stake and join a group with no consequences.
// additionally, it only allows staking a single group at the moment
func StakeTransaction(tree *dag.Dag, transaction *chaintree.Transaction) (newTree *dag.Dag, valid bool, codedErr chaintree.CodedError) {
	payload := &StakePayload{}
	err := typecaster.ToType(transaction.Payload, payload)
	if err != nil {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error casting payload: %v", err)}
	}

	newTree, err = tree.SetAsLink(strings.Split(TreePathForStake, "/"), payload)
	if err != nil {
		return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error setting: %v", err)}
	}

	return newTree, true, nil
}
