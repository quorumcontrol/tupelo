package consensus

import (
	"fmt"
	"strconv"

	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/typecaster"

	cid "github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
)

func init() {
	typecaster.AddType(RemoteNode{})
	cbornode.RegisterCborType(RemoteNode{})
	typecaster.AddType(RoundInfo{})
	cbornode.RegisterCborType(RoundInfo{})
}

// NotaryGroup is a wrapper around a Chain Tree specifically used
// for keeping track of Signer membership and rewards.
type NotaryGroup struct {
	signedTree *SignedChainTree
	ID         string
}

// RoundInfo is a struct that holds information about the round.
// Currently it is only the signers, but is intended to also
// include rewards.
type RoundInfo struct {
	Signers []*RemoteNode
	Round   int64
}

// NewNotaryGroup takes an id and a nodeStore and creates a new empty NotaryGroup
func NewNotaryGroup(id string, nodeStore nodestore.NodeStore) *NotaryGroup {
	tree, err := chaintree.NewChainTree(NewEmptyTree(id, nodeStore), nil, DefaultTransactors)
	if err != nil {
		panic("error creating new tree")
	}
	return &NotaryGroup{
		signedTree: &SignedChainTree{
			ChainTree:  tree,
			Signatures: make(SignatureMap),
		},
		ID: id,
	}
}

// MostRecentRound returns the roundinfo that is most recent to the requested round.
func (ng *NotaryGroup) MostRecentRoundInfo(round int64) (roundInfo *RoundInfo, err error) {
	for i := round; i >= 0; i-- {
		roundInfo, err = ng.RoundInfoFor(i)
		if err != nil {
			return nil, err
		}
		if roundInfo != nil {
			return
		}
	}
	return nil, fmt.Errorf("no valid round found")
}

// RoundInfoFor takes a round and returns the RoundInfo object
func (ng *NotaryGroup) RoundInfoFor(round int64) (roundInfo *RoundInfo, err error) {
	fmt.Printf(ng.signedTree.ChainTree.Dag.Dump())
	obj, _, err := ng.signedTree.ChainTree.Dag.Resolve([]string{"tree", "rounds", strconv.Itoa(int(round))})
	if err != nil {
		return nil, fmt.Errorf("error resolving round nodes: %v", err)
	}
	if obj == nil {
		return nil, nil
	}

	err = typecaster.ToType(obj, &roundInfo)
	if err != nil {
		return nil, fmt.Errorf("error casting resolved obj (%v): %v", obj, err)
	}
	roundInfo.Round = round
	return
}

// FastForward takes the notary group up to a known tip
func (ng *NotaryGroup) FastForward(tip *cid.Cid) error {
	ng.signedTree.ChainTree.Dag = ng.signedTree.ChainTree.Dag.WithNewTip(tip)
	return nil
}

// CreateBlockFor takes a list of remoteNodes and a round and returns a block for gossipping
func (ng *NotaryGroup) CreateBlockFor(round int64, remoteNodes []*RemoteNode) (block *chaintree.BlockWithHeaders, err error) {
	var previousTip string
	if !ng.signedTree.IsGenesis() {
		previousTip = ng.signedTree.Tip().String()
	}
	return &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: previousTip,
			Transactions: []*chaintree.Transaction{
				{
					Type: TransactionTypeSetData,
					Payload: &SetDataPayload{
						Path:  "rounds/" + strconv.Itoa(int(round)),
						Value: RoundInfo{Signers: remoteNodes},
					},
				},
			},
		},
	}, nil
}

// AddBlock takes a block and plays it against the NotaryGroup tree
func (ng *NotaryGroup) AddBlock(block *chaintree.BlockWithHeaders) (err error) {
	valid, err := ng.signedTree.ChainTree.ProcessBlock(block)
	if !valid || err != nil {
		return fmt.Errorf("error processing block (valid: %t): %v", valid, err)
	}
	return nil
}
