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
	cbornode.RegisterCborType(FeedbackRequest{})
	cbornode.RegisterCborType(TipRequest{})
	cbornode.RegisterCborType(TipResponse{})
}

var DidBucket = []byte("tips")
var SigBucket = []byte("sigs")

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
	ChainId   string
	Tip       *cid.Cid
	Signature consensus.Signature
}

type FeedbackRequest struct {
	ChainId   string
	Tip       *cid.Cid
	Signature consensus.Signature
}

type TipRequest struct {
	ChainId string
}

type TipResponse struct {
	ChainId   string
	Tip       *cid.Cid
	Signature consensus.Signature
}

func (s *Signer) SetupStorage() {
	s.Storage.CreateBucketIfNotExists(DidBucket)
	s.Storage.CreateBucketIfNotExists(SigBucket)
}

func (s *Signer) ProcessAddBlock(req *AddBlockRequest) (*AddBlockResponse, error) {

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

	log.Debug("signing", "tip", tip.String())

	sig, err := consensus.BlsSign(tip.Bytes(), s.SignKey)

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
		Signature: *sig,
		ChainId:   id.(string),
	}, nil
}

func (s *Signer) ProcessFeedback(req *FeedbackRequest) error {
	log.Debug("received feedback", "tip", req.Tip.String(), "req", req)

	verified, err := s.Group.VerifySignature(consensus.MustObjToHash(req.Tip.Bytes()), &req.Signature)

	if err != nil {
		return fmt.Errorf("error verifying signature: %v", err)
	}

	if verified {
		sw := &dag.SafeWrap{}
		node := sw.WrapObject(req)
		if sw.Err != nil {
			return fmt.Errorf("error wrapping: %v", sw.Err)
		}

		log.Debug("setting signature", "tip", req.Tip)
		s.Storage.Set(SigBucket, []byte(req.ChainId), node.RawData())
	} else {
		log.Debug("verified", "verified", verified)
		return fmt.Errorf("error, unverified")
	}

	return nil
}

func (s *Signer) ProcessTipRequest(req *TipRequest) (*TipResponse, error) {
	log.Debug("received tip request", "req", req)

	feedbackBytes, err := s.Storage.Get(SigBucket, []byte(req.ChainId))
	if len(feedbackBytes) == 0 || err != nil {
		return nil, fmt.Errorf("error getting chain id: %v", err)
	}

	feedbackRequest := &FeedbackRequest{}
	err = cbornode.DecodeInto(feedbackBytes, feedbackRequest)
	if err != nil {
		return nil, fmt.Errorf("error decoding: %v", err)
	}

	return &TipResponse{
		ChainId:   feedbackRequest.ChainId,
		Tip:       feedbackRequest.Tip,
		Signature: feedbackRequest.Signature,
	}, nil
}
