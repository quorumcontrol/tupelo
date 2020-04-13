package consensus

import (
	"github.com/quorumcontrol/messages/v2/build/go/signatures"

	cid "github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
)

func init() {
	cbornode.RegisterCborType(TipRequest{})
	cbornode.RegisterCborType(TipResponse{})
	cbornode.RegisterCborType(TipSignature{})
	cbornode.RegisterCborType(GetDiffNodesRequest{})
	cbornode.RegisterCborType(GetDiffNodesResponse{})
}

const MessageType_AddBlock = "ADD_BLOCK"
const MessageType_TipRequest = "TIP_REQUEST"
const MessageType_GetDiffNodes = "GET_DIFF_NODES"

type GetDiffNodesRequest struct {
	PreviousTip *cid.Cid
	NewTip      *cid.Cid
}

type GetDiffNodesResponse struct {
	Nodes [][]byte
}

type TipRequest struct {
	ChainId string
}

type TipResponse struct {
	ChainId   string
	Tip       *cid.Cid
	Signature signatures.Signature
}

type TipSignature struct {
	Tip       *cid.Cid
	Signature signatures.Signature
}
