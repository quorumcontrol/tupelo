package notary_test

import (
	"github.com/ethereum/go-ethereum/crypto"
	"testing"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/quorumcontrol/qc3/consensus"
)

var aliceKey,_ = crypto.GenerateKey()
var aliceAddr = crypto.PubkeyToAddress(aliceKey.PublicKey)

var bobKey,_ = crypto.GenerateKey()
var bobAddr = crypto.PubkeyToAddress(bobKey.PublicKey)

var carolKey,_ = crypto.GenerateKey()
var carolAddr = crypto.PubkeyToAddress(carolKey.PublicKey)

func createBlock(t *testing.T, prevBlock *consensuspb.Block) (*consensuspb.Block) {
	retBlock := &consensuspb.Block{
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

	if prevBlock != nil {
		prevHash,err := consensus.BlockToHash(prevBlock)
		if err != nil {
			t.Fatalf("error getting hash of previous block: %v", err)
		}
		retBlock.SignableBlock.PreviousHash = prevHash.Bytes()
	}

	return retBlock
}