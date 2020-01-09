package gossip3to4

import (
	"fmt"

	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/chaintree/typecaster"
	g3services "github.com/quorumcontrol/messages/build/go/services"
	g3signatures "github.com/quorumcontrol/messages/build/go/signatures"
	g4services "github.com/quorumcontrol/messages/v2/build/go/services"
	g4signatures "github.com/quorumcontrol/messages/v2/build/go/signatures"
	"github.com/quorumcontrol/tupelo-go-sdk/consensus"
)

type g3SignatureMap map[string]*g3signatures.Signature
type g3StandardHeaders struct {
	Signatures g3SignatureMap `refmt:"signatures,omitempty" json:"signatures,omitempty" cbor:"signatures,omitempty"`
}

func init() {
	typecaster.AddType(g3StandardHeaders{})
	cbornode.RegisterCborType(g3StandardHeaders{})
	typecaster.AddType(g3signatures.Signature{})
	cbornode.RegisterCborType(g3signatures.Signature{})
}

func ConvertABR(abr *g3services.AddBlockRequest) (*g4services.AddBlockRequest, error) {
	// The thing that changed between gossip3 & gossip4 is in the signature header of the payload
	payloadBytes := abr.Payload

	var payload chaintree.BlockWithHeaders

	err := cbornode.DecodeInto(payloadBytes, &payload)
	if err != nil {
		return nil, fmt.Errorf("error decoding payload into BlockWithHeaders: %v", err)
	}

	headers := &g3StandardHeaders{}
	err = typecaster.ToType(payload.Headers, headers)
	if err != nil {
		return nil, fmt.Errorf("error type casting headers: %v", err)
	}

	sigMap := headers.Signatures

	g4SigMap := make(consensus.SignatureMap)

	for k, sig := range sigMap {
		g4Sig := g4signatures.Signature{
			Ownership: &g4signatures.Ownership{
				PublicKey: &g4signatures.PublicKey{
					Type: g4signatures.PublicKey_KeyTypeSecp256k1,
				},
			},
			Signature: sig.Signature,
		}

		g4SigMap[k] = &g4Sig
	}

	payload.Headers["signatures"] = g4SigMap

	sw := safewrap.SafeWrap{}

	g4PayloadBytes := sw.WrapObject(payload).RawData()
	if sw.Err != nil {
		return nil, fmt.Errorf("error wrapping gossip payload: %v", sw.Err)
	}

	return &g4services.AddBlockRequest{
		ObjectId:    abr.ObjectId,
		PreviousTip: abr.PreviousTip,
		Height:      abr.Height,
		NewTip:      abr.NewTip,
		Payload:     g4PayloadBytes,
		State:       abr.State,
	}, nil
}

