package notary_test

import (
	"crypto/ecdsa"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/quorumcontrol/qc3/notary"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/stretchr/testify/assert"
)

var aliceKey, _ = crypto.GenerateKey()
var aliceAddr = crypto.PubkeyToAddress(aliceKey.PublicKey)

var bobKey, _ = crypto.GenerateKey()
var bobAddr = crypto.PubkeyToAddress(bobKey.PublicKey)

var carolKey, _ = crypto.GenerateKey()
var carolAddr = crypto.PubkeyToAddress(carolKey.PublicKey)

func createBlock(t *testing.T, prevBlock *consensuspb.Block) *consensuspb.Block {
	retBlock := &consensuspb.Block{
		SignableBlock: &consensuspb.SignableBlock{
			Sequence: 0,
			ChainId:  consensus.AddrToDid(aliceAddr.Hex()),
			Transactions: []*consensuspb.Transaction{
				{
					Type:    consensuspb.ADD_DATA,
					Payload: []byte("new data"),
				},
			},
		},
	}

	if prevBlock != nil {
		prevHash, err := consensus.BlockToHash(prevBlock)
		if err != nil {
			t.Fatalf("error getting hash of previous block: %v", err)
		}
		retBlock.SignableBlock.PreviousHash = prevHash.Bytes()
		retBlock.SignableBlock.Sequence = prevBlock.SignableBlock.Sequence + 1
	}

	return retBlock
}

func createBlockWithTransactions(t *testing.T, trans []*consensuspb.Transaction, prevBlock *consensuspb.Block) *consensuspb.Block {
	retBlock := &consensuspb.Block{
		SignableBlock: &consensuspb.SignableBlock{
			Sequence:     0,
			ChainId:      consensus.AddrToDid(aliceAddr.Hex()),
			Transactions: trans,
		},
	}

	if prevBlock != nil {
		prevHash, err := consensus.BlockToHash(prevBlock)
		if err != nil {
			t.Fatalf("error getting hash of previous block: %v", err)
		}
		retBlock.SignableBlock.PreviousHash = prevHash.Bytes()
		retBlock.SignableBlock.Sequence = prevBlock.SignableBlock.Sequence + 1
	}

	return retBlock
}

func createBobBlockWithTransactions(t *testing.T, trans []*consensuspb.Transaction, prevBlock *consensuspb.Block) *consensuspb.Block {
	retBlock := &consensuspb.Block{
		SignableBlock: &consensuspb.SignableBlock{
			Sequence:     0,
			ChainId:      consensus.AddrToDid(bobAddr.Hex()),
			Transactions: trans,
		},
	}

	if prevBlock != nil {
		prevHash, err := consensus.BlockToHash(prevBlock)
		if err != nil {
			t.Fatalf("error getting hash of previous block: %v", err)
		}
		retBlock.SignableBlock.PreviousHash = prevHash.Bytes()
		retBlock.SignableBlock.Sequence = prevBlock.SignableBlock.Sequence + 1
	}

	return retBlock
}

func chainFromEcdsaKey(t *testing.T, key *ecdsa.PublicKey) *consensuspb.Chain {
	return consensus.ChainFromEcdsaKey(key)
}

func defaultNotary(t *testing.T) *notary.Signer {
	key, err := bls.NewSignKey()
	assert.Nil(t, err)

	pubKey := consensus.BlsKeyToPublicKey(key.MustVerKey())
	group := notary.GroupFromPublicKeys([]*consensuspb.PublicKey{pubKey})

	store := notary.NewChainStore("testTips", storage.NewMemStorage())

	return notary.NewSigner(store, group, key)
}
