package signer

import (
	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/ipfs/go-ipld-cbor"
	"fmt"
	"github.com/quorumcontrol/chaintree/typecaster"
	"strings"
)

func init () {
	typecaster.AddType(setDataPayload{})
}

const (
	ErrUnknown = 1
)

var DidBucket = []byte("tips")

type ErrorCode struct {
	Code  int
	Memo string
}

func (e *ErrorCode) GetCode() int {
	return e.Code
}

func (e *ErrorCode) Error() string {
	return fmt.Sprintf("%d - %s", e.Code, e.Memo)
}

type byAddress []*consensus.PublicKey
func (a byAddress) Len() int { return len(a) }
func (a byAddress) Swap(i,j int) { a[i], a[j] = a[j], a[i] }
func (a byAddress) Less(i,j int) bool { return consensus.BlsVerKeyToAddress(a[i].PublicKey).Hex() < consensus.BlsVerKeyToAddress(a[j].PublicKey).Hex() }

var transactors = map[string]chaintree.TransactorFunc{
	"SET_DATA": setData,
}

type Group struct {
	Id string
	SortedPublicKeys []*consensus.PublicKey
}

type Signer struct {
	Id string
	Group      *Group
	Storage storage.Storage
	VerKey     *bls.VerKey
	SignKey    *bls.SignKey
}

type AddBlockRequest struct {
	Nodes [][]byte
	Tip *cid.Cid
	NewBlock *chaintree.BlockWithHeaders
}

type AddBlockResponse struct {
	SignerId string
	Tip *cid.Cid
	Signature []byte
}

func (s *Signer) ProcessRequest(req *AddBlockRequest) (*AddBlockResponse,error) {

	cborNodes := make([]*cbornode.Node, len(req.Nodes))

	sw := &dag.SafeWrap{}

	for i, node := range req.Nodes {
		cborNodes[i] = sw.Decode(node)
	}

	if sw.Err != nil {
		return nil, fmt.Errorf("error decoding: %v", sw.Err)
	}

	tree := dag.NewBidirectionalTree(req.Tip, cborNodes...)

	chainTree,err := chaintree.NewChainTree(
		tree,
		[]chaintree.BlockValidatorFunc{
			s.IsOwner,
		},
		transactors,
	)

	if err != nil {
		return nil, fmt.Errorf("error creating chaintree: %v", err)
	}

	isValid,err := chainTree.ProcessBlock(req.NewBlock)
	if !isValid || err != nil {
		return nil, fmt.Errorf("error processing: %v", err)
	}

	tip := chainTree.Dag.Tip

	sig,err := s.SignKey.Sign(tip.Bytes())

	if err != nil {
		return nil, fmt.Errorf("error signing: %v", err)
	}

	return &AddBlockResponse{
		SignerId: s.Id,
		Tip: tip,
		Signature: sig,
	}, nil
}


func (s *Signer) IsOwner (tree *dag.BidirectionalTree, blockWithHeaders *chaintree.BlockWithHeaders) (bool, chaintree.CodedError) {

	id,_,err := tree.Resolve([]string{"id"})
	if err != nil {
		return false, &ErrorCode{Memo: fmt.Sprintf("error: %v", err), Code: ErrUnknown}
	}

	stored,err := s.Storage.Get(DidBucket, []byte(id.(string)))

	if err != nil {
		return false, &ErrorCode{Memo: fmt.Sprintf("error getting storage: %v", err), Code: ErrUnknown}
	}

	headers := &consensus.StandardHeaders{}

	err = typecaster.ToType(blockWithHeaders.Headers, headers)
	if err != nil {
		return false, &ErrorCode{Memo: fmt.Sprintf("error: %v", err), Code: ErrUnknown}
	}

	if stored == nil {
		// this is a genesis block
		addr := consensus.DidToAddr(id.(string))
		isSigned,err := consensus.IsBlockSignedBy(blockWithHeaders, addr)

		if err != nil {
			return false, &ErrorCode{Memo: fmt.Sprintf("error finding if signed: %v", err), Code: ErrUnknown}
		}

		if isSigned {
			return true, nil
		} else {
			return false, nil
		}
	} else {
		return false, &ErrorCode{Memo: "unimplemented"}
	}

}


type setDataPayload struct {
	Path string
	Value interface{}
}


func setData(tree *dag.BidirectionalTree, transaction *chaintree.Transaction) (valid bool, codedErr chaintree.CodedError) {
	payload := &setDataPayload{}
	err := typecaster.ToType(transaction.Payload, payload)
	if err != nil {
		return false, &ErrorCode{Code: ErrUnknown, Memo: fmt.Sprintf("error casting payload: %v", err)}
	}

	err = tree.Set(strings.Split(payload.Path, "/"), payload.Value)
	if err != nil {
		return false, &ErrorCode{Code: 999, Memo: fmt.Sprintf("error setting: %v", err)}
	}

	return true, nil
}
