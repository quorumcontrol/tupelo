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
	TreePathForCoins           = "_qc/coins"
	TreePathForStake           = "_qc/stake"
)

func init() {
	typecaster.AddType(SetDataPayload{})
	typecaster.AddType(SetOwnershipPayload{})
	typecaster.AddType(EstablishCoinPayload{})
	typecaster.AddType(CoinMonetaryPolicy{})
	typecaster.AddType(MintCoinPayload{})
	typecaster.AddType(StakePayload{})
	cbornode.RegisterCborType(SetDataPayload{})
	cbornode.RegisterCborType(SetOwnershipPayload{})
	cbornode.RegisterCborType(EstablishCoinPayload{})
	cbornode.RegisterCborType(CoinMonetaryPolicy{})
	cbornode.RegisterCborType(MintCoinPayload{})
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

type SetOwnershipPayload struct {
	Authentication []*PublicKey
}

// SetOwnershipTransaction changes the ownership of a tree by adding a public key array to /_qc/authentications
func SetOwnershipTransaction(tree *dag.Dag, transaction *chaintree.Transaction) (newTree *dag.Dag, valid bool, codedErr chaintree.CodedError) {
	payload := &SetOwnershipPayload{}
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
	existingCoin, _, err := tree.Resolve(coinPath)
	if err != nil {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error attempting to resolve %v: %v", coinPath, err)}
	}
	if existingCoin != nil {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error, coin at path %v already exists", coinPath)}
	}

	coinData := map[string]interface{}{
		"mints":    nil,
		"sends":    nil,
		"receives": nil,
	}

	newTree, err = tree.SetAsLink(coinPath, coinData)
	if err != nil {
		return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error setting: %v", err)}
	}

	newTree, err = tree.SetAsLink(append(coinPath, "monetaryPolicy"), payload.MonetaryPolicy)
	if err != nil {
		return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error setting: %v", err)}
	}

	return newTree, true, nil
}

type MintCoinPayload struct {
	Name   string
	Amount uint64
}

func MintCoinTransaction(tree *dag.Dag, transaction *chaintree.Transaction) (newTree *dag.Dag, valid bool, codedErr chaintree.CodedError) {
	payload := &MintCoinPayload{}
	err := typecaster.ToType(transaction.Payload, payload)

	if err != nil {
		return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error setting: %v", err)}
	}

	if payload.Amount <= 0 {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: "error, can not mint an amount < 0"}
	}

	coinName := payload.Name
	coinPath := append(strings.Split(TreePathForCoins, "/"), coinName)

	uncastMonetaryPolicy, _, err := tree.Resolve(append(coinPath, "monetaryPolicy"))
	if err != nil {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error fetch coin at path %v: %v", coinPath, err)}
	}
	if uncastMonetaryPolicy == nil {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error, coin at path %v does not exist, must MINT_COIN first", coinPath)}
	}
	monetaryPolicy, ok := uncastMonetaryPolicy.(map[string]interface{})
	if !ok {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: "error on type assertion for monetaryPolicy"}
	}

	uncastMintCids, _, err := tree.Resolve(append(coinPath, "mints"))
	if err != nil {
		return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error fetching mints at %v: %v", coinPath, err)}
	}

	var mintCids []cid.Cid

	if uncastMintCids == nil {
		mintCids = make([]cid.Cid, 0)
	} else {
		mintCids = make([]cid.Cid, len(uncastMintCids.([]interface{})))
		for k, c := range uncastMintCids.([]interface{}) {
			mintCids[k] = c.(cid.Cid)
		}
	}

	if monetaryPolicy["maximum"].(uint64) > 0 {
		var currentMintedTotal uint64

		for _, c := range mintCids {
			node, err := tree.Get(&c)

			if err != nil {
				return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error fetching node %v: %v", c, err)}
			}

			amount, _, err := node.Resolve([]string{"amount"})

			if err != nil {
				return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error fetching amount from %v: %v", node, err)}
			}

			currentMintedTotal = currentMintedTotal + amount.(uint64)
		}

		if (currentMintedTotal + payload.Amount) >= monetaryPolicy["maximum"].(uint64) {
			return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("new mint would violate monetaryPolicy of maximum: %v", monetaryPolicy["maximum"])}
		}
	}

	newMint, err := tree.CreateNode(map[string]interface{}{
		"amount": payload.Amount,
	})
	if err != nil {
		return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("could not create new node: %v", err)}
	}

	mintCids = append(mintCids, *newMint.Cid())

	newTree, err = tree.SetAsLink(append(coinPath, "mints"), mintCids)
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
