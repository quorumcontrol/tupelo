package consensus

import (
	"crypto/ecdsa"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
)

func init() {
	cbornode.RegisterCborType(AddBlockRequest{})
	cbornode.RegisterCborType(AddBlockResponse{})
	cbornode.RegisterCborType(TipRequest{})
	cbornode.RegisterCborType(TipResponse{})
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

type AddBlockRequest struct {
	ChainId  string
	Nodes    [][]byte
	Tip      *cid.Cid
	NewTip   *cid.Cid
	NewBlock *chaintree.BlockWithHeaders
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

type Signature struct {
	Signers   []bool `refmt:"signers,omitempty" json:"signers,omitempty" cbor:"signers,omitempty"`
	Signature []byte `refmt:"signature,omitempty" json:"signature,omitempty" cbor:"signature,omitempty"`
	Type      string `refmt:"type,omitempty" json:"type,omitempty" cbor:"type,omitempty"`
}

type TipRequest struct {
	ChainId string
}

type TipResponse struct {
	ChainId     string
	Tip         *cid.Cid
	PreviousTip *cid.Cid
	Cycle       uint64
	Epoch       uint64
	Signature   Signature
}
