package consensus_test

import (
	"github.com/ethereum/go-ethereum/crypto"
	"testing"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/quorumcontrol/qc3/consensus"
)

var aliceKey,_ = crypto.GenerateKey()
var aliceAddr = crypto.PubkeyToAddress(aliceKey.PublicKey)

func genValidGenesisBlock(t *testing.T) (*consensuspb.Block) {
	return &consensuspb.Block{
		SignableBlock: &consensuspb.SignableBlock{
			ChainId: consensus.AddrToDid(aliceAddr.Hex()),
			Transactions: []*consensuspb.Transaction{
				{
					Type: consensuspb.ADD_DATA,
					Payload: []byte("new data"),
				},
			},
		},
	}
}