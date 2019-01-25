package consensus

import (
	"fmt"
	"strings"

	cid "github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/typecaster"
)

const (
	TreePathForAuthentications = "_tupelo/authentications"
	TreePathForCoins           = "_tupelo/coins"
	TreePathForStake           = "_tupelo/stake"
)

func init() {
	typecaster.AddType(SetDataPayload{})
	typecaster.AddType(SetOwnershipPayload{})
	typecaster.AddType(EstablishCoinPayload{})
	typecaster.AddType(MintCoinPayload{})
	typecaster.AddType(Coin{})
	typecaster.AddType(CoinMonetaryPolicy{})
	typecaster.AddType(CoinMint{})
	typecaster.AddType(StakePayload{})
	cbornode.RegisterCborType(SetDataPayload{})
	cbornode.RegisterCborType(SetOwnershipPayload{})
	cbornode.RegisterCborType(EstablishCoinPayload{})
	cbornode.RegisterCborType(MintCoinPayload{})
	cbornode.RegisterCborType(Coin{})
	cbornode.RegisterCborType(CoinMonetaryPolicy{})
	cbornode.RegisterCborType(CoinMint{})
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

func DecodePath(path string) ([]string, error) {
	trimmed := strings.TrimPrefix(path, "/")
	split := strings.Split(trimmed, "/")
	if len(split) > 1 { // []string{""} is the only valid path with an empty string
		for _, component := range split {
			if component == "" {
				return nil, fmt.Errorf("malformed path string containing repeated separator: %s", path)
			}
		}
	}
	return split, nil
}

// SetDataTransaction just sets a path in a tree to arbitrary data. It makes sure no data is being changed in the _tupelo path.
func SetDataTransaction(tree *dag.Dag, transaction *chaintree.Transaction) (newTree *dag.Dag, valid bool, codedErr chaintree.CodedError) {
	payload := &SetDataPayload{}
	err := typecaster.ToType(transaction.Payload, payload)
	if err != nil {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error casting payload: %v", err)}
	}

	path, err := DecodePath(payload.Path)
	if err != nil {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error decoding path: %v", err)}
	}

	if path[0] == "_tupelo" {
		return nil, false, &ErrorCode{Code: 999, Memo: "the path prefix _tupelo is reserved"}
	}

	if complexType(payload.Value) {
		newTree, err = tree.SetAsLink(path, payload.Value)
	} else {
		newTree, err = tree.Set(path, payload.Value)
	}
	if err != nil {
		return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error setting: %v", err)}
	}

	return newTree, true, nil
}

type SetOwnershipPayload struct {
	Authentication []string
}

// SetOwnershipTransaction changes the ownership of a tree by adding a public key array to /_tupelo/authentications
func SetOwnershipTransaction(tree *dag.Dag, transaction *chaintree.Transaction) (newTree *dag.Dag, valid bool, codedErr chaintree.CodedError) {
	payload := &SetOwnershipPayload{}
	err := typecaster.ToType(transaction.Payload, payload)
	if err != nil {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error casting payload: %v", err)}
	}

	path, err := DecodePath(TreePathForAuthentications)
	if err != nil {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error decoding path: %v", err)}
	}

	newTree, err = tree.SetAsLink(path, payload.Authentication)
	if err != nil {
		return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error setting: %v", err)}
	}

	return newTree, true, nil
}

type Coin struct {
	MonetaryPolicy *cid.Cid
	Mints          *cid.Cid
	Sends          *cid.Cid
	Receives       *cid.Cid
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
	path, err := DecodePath(TreePathForCoins)
	if err != nil {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error decoding path: %v", err)}
	}
	coinPath := append(path, coinName)
	existingCoin, _, err := tree.Resolve(coinPath)
	if err != nil {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error attempting to resolve %v: %v", coinPath, err)}
	}
	if existingCoin != nil {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error, coin at path %v already exists", coinPath)}
	}

	newTree, err = tree.SetAsLink(coinPath, &Coin{})
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

type CoinMint struct {
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
	path, err := DecodePath(TreePathForCoins)
	if err != nil {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error decoding path: %v", err)}
	}
	coinPath := append(path, coinName)

	uncastMonetaryPolicy, _, err := tree.Resolve(append(coinPath, "monetaryPolicy"))
	if err != nil {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error fetch coin at path %v: %v", coinPath, err)}
	}
	if uncastMonetaryPolicy == nil {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error, coin at path %v does not exist, must MINT_COIN first", coinPath)}
	}

	monetaryPolicy := &CoinMonetaryPolicy{}
	err = typecaster.ToType(uncastMonetaryPolicy, monetaryPolicy)

	if err != nil {
		return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error setting: %v", err)}
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

	if monetaryPolicy.Maximum > 0 {
		var currentMintedTotal uint64

		for _, c := range mintCids {
			node, err := tree.Get(c)

			if err != nil {
				return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error fetching node %v: %v", c, err)}
			}

			amount, _, err := node.Resolve([]string{"amount"})

			if err != nil {
				return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error fetching amount from %v: %v", node, err)}
			}

			currentMintedTotal = currentMintedTotal + amount.(uint64)
		}

		if (currentMintedTotal + payload.Amount) >= monetaryPolicy.Maximum {
			return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("new mint would violate monetaryPolicy of maximum: %v", monetaryPolicy.Maximum)}
		}
	}

	newMint, err := tree.CreateNode(&CoinMint{
		Amount: payload.Amount,
	})
	if err != nil {
		return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("could not create new node: %v", err)}
	}

	mintCids = append(mintCids, newMint.Cid())

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

	path, err := DecodePath(TreePathForStake)
	if err != nil {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error decoding path: %v", err)}
	}

	newTree, err = tree.SetAsLink(path, payload)
	if err != nil {
		return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error setting: %v", err)}
	}

	return newTree, true, nil
}
