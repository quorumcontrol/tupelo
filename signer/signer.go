package signer

import (
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/typecaster"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/storage"
)

func init() {
	typecaster.AddType(AddBlockResponse{})
	typecaster.AddType(AddBlockRequest{})
	cbornode.RegisterCborType(AddBlockRequest{})
	cbornode.RegisterCborType(AddBlockResponse{})
}

var DidBucket = []byte("tips")

type Signer struct {
	Id      string
	Group   *consensus.Group
	Storage storage.Storage
	VerKey  *bls.VerKey
	SignKey *bls.SignKey
}

type AddBlockRequest struct {
	Nodes    [][]byte
	Tip      *cid.Cid
	NewBlock *chaintree.BlockWithHeaders
}

type AddBlockResponse struct {
	SignerId  string
	Tip       *cid.Cid
	Signature []byte
}

func (s *Signer) SetupStorage() {
	s.Storage.CreateBucketIfNotExists(DidBucket)
}

func (s *Signer) ProcessRequest(req *AddBlockRequest) (*AddBlockResponse, error) {

	cborNodes := make([]*cbornode.Node, len(req.Nodes))

	sw := &dag.SafeWrap{}

	for i, node := range req.Nodes {
		cborNodes[i] = sw.Decode(node)
	}

	if sw.Err != nil {
		return nil, fmt.Errorf("error decoding: %v", sw.Err)
	}

	log.Debug("received: ", "tip", req.Tip, "len(nodes)", len(cborNodes))

	tree := dag.NewBidirectionalTree(req.Tip, cborNodes...)

	chainTree, err := chaintree.NewChainTree(
		tree,
		[]chaintree.BlockValidatorFunc{
			s.IsOwner,
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

	sig, err := s.SignKey.Sign(tip.Bytes())

	if err != nil {
		return nil, fmt.Errorf("error signing: %v", err)
	}

	id, _, err := tree.Resolve([]string{"id"})
	if err != nil {
		return nil, &consensus.ErrorCode{Memo: fmt.Sprintf("error: %v", err), Code: consensus.ErrUnknown}
	}

	s.Storage.Set(DidBucket, []byte(id.(string)), tip.Bytes())

	return &AddBlockResponse{
		SignerId:  s.Id,
		Tip:       tip,
		Signature: sig,
	}, nil
}
