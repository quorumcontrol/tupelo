package signer

import (
	"fmt"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/chaintree/typecaster"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/storage"
)

func init() {
	typecaster.AddType(consensus.AddBlockResponse{})
	typecaster.AddType(consensus.AddBlockRequest{})
}

func processAddBlock(currentTip cid.Cid, req *consensus.AddBlockRequest) (*consensus.AddBlockResponse, error) {
	log.Debug("process add block", "storedTip", currentTip)
	if !currentTip.Defined() {
		if req.NewBlock.PreviousTip != "" {
			log.Error("unmatching tips", "currentTip", "nil", "sent", req.Tip.String())
			return nil, &consensus.ErrorCode{Memo: "invalid tip", Code: consensus.ErrInvalidTip}
		}
		currentTip = *req.Tip
	} else {
		if !currentTip.Equals(*req.Tip) {
			log.Error("unmatching tips", "currentTip", currentTip.String(), "sent", req.Tip.String())
			return nil, &consensus.ErrorCode{Memo: "unknown tip", Code: consensus.ErrInvalidTip}
		}
	}

	cborNodes := make([]*cbornode.Node, len(req.Nodes))

	sw := &safewrap.SafeWrap{}

	for i, node := range req.Nodes {
		cborNodes[i] = sw.Decode(node)
	}

	if sw.Err != nil {
		return nil, fmt.Errorf("error decoding: %v", sw.Err)
	}

	log.Debug("received: ", "tip", req.Tip, "nodeLength", len(cborNodes))

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
		return nil, fmt.Errorf("error creating chaintree: %v", err)
	}

	isValid, err := chainTree.ProcessBlock(req.NewBlock)
	if !isValid || err != nil {
		return nil, fmt.Errorf("error processing: %v", err)
	}

	tip := chainTree.Dag.Tip

	id, _, err := tree.Resolve([]string{"id"})
	if err != nil {
		return nil, &consensus.ErrorCode{Memo: fmt.Sprintf("error: %v", err), Code: consensus.ErrUnknown}
	}
	log.Debug("signer stating new tip", "tip", tip.String())

	return &consensus.AddBlockResponse{
		Tip:     &tip,
		ChainId: id.(string),
	}, nil
}
