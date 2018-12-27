package gossipclient

import (
	"crypto/ecdsa"
	"time"

	"github.com/quorumcontrol/tupelo/consensus"
)

type GossipClient interface {
	Subscribe(signerDstKey *ecdsa.PublicKey, did string, timeout time.Duration) (respChan chan *consensus.TipResponse, err error)
	AddBlock(signerDstKey *ecdsa.PublicKey, request *consensus.AddBlockRequest) error
	GetTip(signerDstKey *ecdsa.PublicKey, request *consensus.TipRequest) (*consensus.TipResponse, error)
}
