package consensus

import (
	"fmt"
	"strings"

	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/typecaster"
)

const (
	TreePathForAuthentications = "_qc/authentications"
	TreePathForCoins           = "_qc/coins"
	TreePathForStake           = "_qc/stake"
)

func init() {
	typecaster.AddType(SetDataPayload{})
	typecaster.AddType(setOwnershipPayload{})
	typecaster.AddType(EstablishCoinPayload{})
	typecaster.AddType(CoinMonetaryPolicy{})
	typecaster.AddType(StakePayload{})
	cbornode.RegisterCborType(SetDataPayload{})
	cbornode.RegisterCborType(setOwnershipPayload{})
	cbornode.RegisterCborType(EstablishCoinPayload{})
	cbornode.RegisterCborType(CoinMonetaryPolicy{})
	cbornode.RegisterCborType(StakePayload{})
}

// SetDataPayload is the payload for a SetDataTransaction
// Path / Value
type SetDataPayload struct {
	Path  string
	Value interface{}
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

	newTree, err = tree.Set(strings.Split(payload.Path, "/"), payload.Value)
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

type CoinMonetaryPolicy struct {
	Maximum uint64
}

type EstablishCoinPayload struct {
	Name           string
	MonetaryPolicy CoinMonetaryPolicy
}

func EstablishCoinTransaction(tree *dag.Dag, transaction *chaintree.Transaction) (newTree *dag.Dag, valid bool, codedErr chaintree.CodedError) {
	payload := &EstablishCoinPayload{}
	err := typecaster.ToType(transaction.Payload, payload)

	if err != nil {
		return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error setting: %v", err)}
	}

	coinName := payload.Name
	coinPath := append(strings.Split(TreePathForCoins, "/"), coinName)

	coinExists, _, _ := tree.Resolve(coinPath)
	if coinExists != nil {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error, coin at path %v already exists", coinPath)}
	}

	coinData := map[string]interface{}{
		"mints":    nil,
		"sends":    nil,
		"receives": nil,
	}

	newTree, err = tree.SetAsLink(coinPath, coinData)
	if err != nil {
		return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error setting 4: %v", err)}
	}

	newTree, err = tree.SetAsLink(append(coinPath, "monetaryPolicy"), payload.MonetaryPolicy)
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
