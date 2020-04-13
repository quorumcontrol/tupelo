package client

import (
	"context"
	"crypto/ecdsa"
	"fmt"

	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	format "github.com/ipfs/go-ipld-format"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/messages/v2/build/go/transactions"
	"github.com/quorumcontrol/tupelo/sdk/consensus"
)

func (c *Client) NewAddBlockRequest(ctx context.Context, tree *consensus.SignedChainTree, treeKey *ecdsa.PrivateKey, transactions []*transactions.Transaction) (*services.AddBlockRequest, error) {
	height, err := getHeight(ctx, tree)
	if err != nil {
		return nil, fmt.Errorf("error getting tree height: %v", err)
	}

	treeTip := tree.Tip()

	var blockTip *cid.Cid
	if !tree.IsGenesis() {
		blockTip = &treeTip
	}

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			Height:       height,
			PreviousTip:  blockTip,
			Transactions: transactions,
		},
	}

	blockWithHeaders, err := consensus.SignBlock(ctx, unsignedBlock, treeKey)
	if err != nil {
		return nil, fmt.Errorf("error signing block: %w", err)
	}

	if len(tree.ChainTree.BlockValidators) == 0 {
		// we run the block validators to save devs from themselves
		// and catch anything we know will be rejected by the NotaryGroup
		validators, err := c.Group.BlockValidators(ctx)
		if err != nil {
			return nil, fmt.Errorf("error getting notary group block validators: %v", err)
		}
		tree.ChainTree.BlockValidators = validators
	}

	trackedTree, tracker, err := refTrackingChainTree(ctx, tree.ChainTree)
	if err != nil {
		return nil, fmt.Errorf("error creating reference tracker: %v", err)
	}

	valid, err := trackedTree.ProcessBlock(ctx, blockWithHeaders)
	if !valid || err != nil {
		return nil, fmt.Errorf("error processing block (valid: %t): %v", valid, err)
	}

	var state [][]byte
	// only need state after the first Tx
	if blockWithHeaders.Height > 0 {
		// Grab the nodes that were actually used:
		touchedNodes, err := tracker.touchedNodes(ctx)
		if err != nil {
			return nil, fmt.Errorf("error getting node: %v", err)
		}
		state = append(nodesToBytes(touchedNodes))
	}

	// TODO: sending in the *new* nodes is an expedient way to make sure the signers bitswap our new state for us
	// but it will cost more in transaction fees and so should be rexamined in the future.
	newNodes, err := tracker.newNodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting node: %v", err)
	}
	state = append(state, nodesToBytes(newNodes)...)

	expectedTip := trackedTree.Dag.Tip
	chainId, err := tree.Id()
	if err != nil {
		return nil, err
	}

	sw := safewrap.SafeWrap{}
	payload := sw.WrapObject(blockWithHeaders).RawData()

	return &services.AddBlockRequest{
		PreviousTip: treeTip.Bytes(),
		Height:      blockWithHeaders.Height,
		Payload:     payload,
		NewTip:      expectedTip.Bytes(),
		ObjectId:    []byte(chainId),
		State:       state,
	}, nil
}

// Creating a new dag & chaintree with a tracked datastore to ensure that only
// the necessary nodes are sent to the signers for a processed block. Processing
// blocks on the tracked chaintree will keep track of which nodes are accessed.
func refTrackingChainTree(ctx context.Context, untrackedTree *chaintree.ChainTree) (*chaintree.ChainTree, *storeWrapper, error) {
	originalStore := untrackedTree.Dag.Store
	tracker := wrapStoreForRefCounting(originalStore)
	trackedTree, err := chaintree.NewChainTree(ctx, dag.NewDag(ctx, untrackedTree.Dag.Tip, tracker), untrackedTree.BlockValidators, untrackedTree.Transactors)

	return trackedTree, tracker, err
}

func getHeight(ctx context.Context, tree *consensus.SignedChainTree) (uint64, error) {
	ct := tree.ChainTree

	unmarshaledRoot, err := ct.Dag.Get(ctx, ct.Dag.Tip)
	if unmarshaledRoot == nil || err != nil {
		return 0, fmt.Errorf("error, missing root: %v", err)
	}

	root := &chaintree.RootNode{}

	err = cbornode.DecodeInto(unmarshaledRoot.RawData(), root)
	if err != nil {
		return 0, fmt.Errorf("error decoding root: %v", err)
	}

	var height uint64
	if tree.IsGenesis() {
		height = 0
	} else {
		height = root.Height + 1
	}

	return height, nil
}

func nodesToBytes(nodes []format.Node) [][]byte {
	returnBytes := make([][]byte, len(nodes))
	for i, n := range nodes {
		returnBytes[i] = n.RawData()
	}

	return returnBytes
}
