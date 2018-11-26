package gossip2

import (
	"context"
	"fmt"

	cid "github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/quorumcontrol/storage"
)

func chainTreeStateHandler(ctx context.Context, stateTrans StateTransaction) (nextState []byte, accepted bool, err error) {
	var currentTip cid.Cid
	if len(stateTrans.CurrentState) > 1 {
		currentTip, err = cid.Cast(stateTrans.CurrentState)
		if err != nil {
			return nil, false, fmt.Errorf("error casting CID: %v", err)
		}
	}

	addBlockrequest := &consensus.AddBlockRequest{}
	err = cbornode.DecodeInto(stateTrans.Transaction, addBlockrequest)
	if err != nil {
		return nil, false, fmt.Errorf("error getting payload: %v", err)
	}

	if currentTip.Defined() {
		if !currentTip.Equals(*addBlockrequest.Tip) {
			log.Errorf("unmatching tips %s, %s", currentTip.String(), addBlockrequest.Tip.String())
			return nil, false, &consensus.ErrorCode{Memo: "unknown tip", Code: consensus.ErrInvalidTip}
		}
	} else {
		currentTip = *addBlockrequest.Tip
	}

	cborNodes := make([]*cbornode.Node, len(addBlockrequest.Nodes))

	sw := &safewrap.SafeWrap{}

	for i, node := range addBlockrequest.Nodes {
		cborNodes[i] = sw.Decode(node)
	}

	if sw.Err != nil {
		return nil, false, fmt.Errorf("error decoding: %v", sw.Err)
	}
	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	tree := dag.NewDag(currentTip, nodeStore)
	tree.AddNodes(cborNodes...)

	chainTree, err := chaintree.NewChainTree(
		tree,
		[]chaintree.BlockValidatorFunc{
			isOwner,
		},
		consensus.DefaultTransactors,
	)

	if err != nil {
		return nil, false, fmt.Errorf("error creating chaintree: %v", err)
	}

	isValid, err := chainTree.ProcessBlock(addBlockrequest.NewBlock)
	if !isValid || err != nil {
		return nil, false, fmt.Errorf("error processing: %v", err)
	}

	return chainTree.Dag.Tip.Bytes(), true, nil
}
