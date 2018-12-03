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
	TreePathForAuthentications = "_tupelo/authentications"
	TreePathForCoins           = "_tupelo/coins"
	TreePathForStake           = "_tupelo/stake"
)

func init() {
	typecaster.AddType(SetDataPayload{})
	typecaster.AddType(SetOwnershipPayload{})
	typecaster.AddType(EstablishCoinPayload{})
	typecaster.AddType(MintCoinPayload{})
	typecaster.AddType(SendCoinPayload{})
	typecaster.AddType(Coin{})
	typecaster.AddType(CoinMonetaryPolicy{})
	typecaster.AddType(CoinMint{})
	typecaster.AddType(CoinSend{})
	typecaster.AddType(StakePayload{})
	cbornode.RegisterCborType(SetDataPayload{})
	cbornode.RegisterCborType(SetOwnershipPayload{})
	cbornode.RegisterCborType(EstablishCoinPayload{})
	cbornode.RegisterCborType(MintCoinPayload{})
	cbornode.RegisterCborType(SendCoinPayload{})
	cbornode.RegisterCborType(Coin{})
	cbornode.RegisterCborType(CoinMonetaryPolicy{})
	cbornode.RegisterCborType(CoinMint{})
	cbornode.RegisterCborType(CoinSend{})
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

// SetDataTransaction just sets a path in a tree to arbitrary data. It makes sure no data is being changed in the _tupelo path.
func SetDataTransaction(tree *dag.Dag, transaction *chaintree.Transaction) (newTree *dag.Dag, valid bool, codedErr chaintree.CodedError) {
	payload := &SetDataPayload{}
	err := typecaster.ToType(transaction.Payload, payload)
	if err != nil {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error casting payload: %v", err)}
	}

	if payload.Path == "_tupelo" || strings.HasPrefix(payload.Path, "_tupelo/") {
		return nil, false, &ErrorCode{Code: 999, Memo: "the path prefix _tupelo is reserved"}
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

type SetOwnershipPayload struct {
	Authentication []*PublicKey
}

// SetOwnershipTransaction changes the ownership of a tree by adding a public key array to /_tupelo/authentications
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
	coinPath := append(strings.Split(TreePathForCoins, "/"), coinName)
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
	coinPath := append(strings.Split(TreePathForCoins, "/"), coinName)

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

type SendCoinPayload struct {
	Id          string
	Name        string
	Amount      uint64
	Destination string
}
type CoinSend struct {
	Id          string
	Amount      uint64
	Destination string
}

func SendCoinTransaction(tree *dag.Dag, transaction *chaintree.Transaction) (newTree *dag.Dag, valid bool, codedErr chaintree.CodedError) {
	payload := &SendCoinPayload{}
	err := typecaster.ToType(transaction.Payload, payload)

	// TODO: verify recipient is chaintree address?

	if err != nil {
		return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error setting: %v", err)}
	}

	if payload.Amount <= 0 {
		return nil, false, &ErrorCode{Code: ErrUnknown, Memo: "error, must send an amount greater than 0"}
	}

	coinName := payload.Name
	coinPath := append(strings.Split(TreePathForCoins, "/"), coinName)

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

	var availableBalance uint64

	for _, c := range mintCids {
		node, err := tree.Get(c)

		if err != nil {
			return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error fetching node %v: %v", c, err)}
		}

		amount, _, err := node.Resolve([]string{"amount"})

		if err != nil {
			return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error fetching amount from %v: %v", node, err)}
		}

		availableBalance = availableBalance + amount.(uint64)
	}

	uncastReceivedCids, _, err := tree.Resolve(append(coinPath, "receives"))
	if err != nil {
		return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error fetching receives at %v: %v", coinPath, err)}
	}

	var receivedCids []cid.Cid

	if uncastReceivedCids == nil {
		receivedCids = make([]cid.Cid, 0)
	} else {
		receivedCids = make([]cid.Cid, len(uncastReceivedCids.([]interface{})))
		for k, c := range uncastReceivedCids.([]interface{}) {
			receivedCids[k] = c.(cid.Cid)
		}
	}

	for _, c := range receivedCids {
		node, err := tree.Get(c)

		if err != nil {
			return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error fetching node %v: %v", c, err)}
		}

		amount, _, err := node.Resolve([]string{"amount"})

		if err != nil {
			return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error fetching amount from %v: %v", node, err)}
		}
		availableBalance = availableBalance + amount.(uint64)
	}

	uncastSentCids, _, err := tree.Resolve(append(coinPath, "sends"))
	if err != nil {
		return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error fetching sends at %v: %v", coinPath, err)}
	}

	var sentCids []cid.Cid

	if uncastSentCids == nil {
		sentCids = make([]cid.Cid, 0)
	} else {
		sentCids = make([]cid.Cid, len(uncastSentCids.([]interface{})))
		for k, c := range uncastSentCids.([]interface{}) {
			sentCids[k] = c.(cid.Cid)
		}
	}

	for _, c := range sentCids {
		node, err := tree.Get(c)

		if err != nil {
			return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error fetching node %v: %v", c, err)}
		}

		amount, _, err := node.Resolve([]string{"amount"})

		if err != nil {
			return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error fetching amount from %v: %v", node, err)}
		}
		availableBalance = availableBalance - amount.(uint64)
	}

	if availableBalance < payload.Amount {
		return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("can not send coin, balance of %d is too low to send %d", availableBalance, payload.Amount)}
	}

	newSend, err := tree.CreateNode(&CoinSend{
		Id:          payload.Id,
		Amount:      payload.Amount,
		Destination: payload.Destination,
	})
	if err != nil {
		return nil, false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("could not create new node: %v", err)}
	}

	sentCids = append(sentCids, newSend.Cid())

	newTree, err = tree.SetAsLink(append(coinPath, "sends"), sentCids)
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
