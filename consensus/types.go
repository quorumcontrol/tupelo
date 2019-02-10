package consensus

import (
	"crypto/ecdsa"

	cid "github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
)

func init() {
	cbornode.RegisterCborType(AddBlockResponse{})
	cbornode.RegisterCborType(FeedbackRequest{})
	cbornode.RegisterCborType(TipRequest{})
	cbornode.RegisterCborType(TipResponse{})
	cbornode.RegisterCborType(TipSignature{})
	cbornode.RegisterCborType(GetDiffNodesRequest{})
	cbornode.RegisterCborType(GetDiffNodesResponse{})
}

const MessageType_AddBlock = "ADD_BLOCK"
const MessageType_Feedback = "FEEDBACK"
const MessageType_TipRequest = "TIP_REQUEST"
const MessageType_GetDiffNodes = "GET_DIFF_NODES"

type Wallet interface {
	GetChain(id string) (*SignedChainTree, error)
	SaveChain(chain *SignedChainTree) error
	//Balances(id string) (map[string]uint64, error)
	GetChainIds() ([]string, error)
	Close()

	GetKey(addr string) (*ecdsa.PrivateKey, error)
	GenerateKey() (*ecdsa.PrivateKey, error)
	ListKeys() ([]string, error)
}

type AddBlockResponse struct {
	SignerId  string
	ChainId   string
	Tip       *cid.Cid
	Signature Signature
}

type GetDiffNodesRequest struct {
	PreviousTip *cid.Cid
	NewTip      *cid.Cid
}

type GetDiffNodesResponse struct {
	Nodes [][]byte
}

type FeedbackRequest struct {
	ChainId   string
	Tip       *cid.Cid
	Signature Signature
}

type TipRequest struct {
	ChainId string
}

type TipResponse struct {
	ChainId   string
	Tip       *cid.Cid
	Signature Signature
}

type TipSignature struct {
	Tip       *cid.Cid
	Signature Signature
}
