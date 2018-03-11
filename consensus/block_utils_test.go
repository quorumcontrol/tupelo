package consensus_test

import (
	"testing"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"crypto/ecdsa"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/stretchr/testify/assert"
	"github.com/quorumcontrol/qc3/internalchain"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/common"
)

func TestOwnerSignBlock(t *testing.T) {
	type testDescription struct {
		Description        string
		Block         *consensuspb.Block
		PrivateKey *ecdsa.PrivateKey
		ShouldError        bool
	}
	type testGenerator func(t *testing.T) (*testDescription)

	for _,testGen := range []testGenerator{
		func(t *testing.T) (*testDescription) {
			return &testDescription{
				Description: "valid everything",
				Block:       createBlock(t, nil),
				PrivateKey:  aliceKey,
				ShouldError: false,
			}
		},
	} {
		test := testGen(t)
		sigLength := len(test.Block.Signatures)
		blockWithSig,err := consensus.OwnerSignBlock(test.Block, test.PrivateKey)
		if test.ShouldError {
			assert.NotNil(t, err, test.Description)
		} else {
			assert.Nil(t, err, test.Description)
		}
		assert.Equal(t, len(blockWithSig.Signatures), sigLength + 1, test.Description)
		assert.Equal(t, blockWithSig.Signatures[len(blockWithSig.Signatures) - 1].Creator, aliceAddr.Hex())
	}
}

func TestSanity(t *testing.T) {
	hsh := common.BytesToHash([]byte("data"))
	hshBytes := hsh.Bytes()
	assert.Len(t, hsh.Bytes(), 32)
	assert.Equal(t, hsh.Bytes(), hsh.Bytes())

	sig,err := crypto.Sign(hshBytes, aliceKey)
	assert.Nil(t, err)

	pubKeyFromSig,err := crypto.Ecrecover(hsh.Bytes(), sig)
	assert.Nil(t, err)

	valid := crypto.VerifySignature(pubKeyFromSig, hshBytes, sig[:len(sig)-1])
	assert.True(t, valid)
}

func TestVerifySignature(t *testing.T) {
	type testDescription struct {
		Description        string
		Block         *consensuspb.Block
		InternalOwnership *internalchain.InternalOwnership
		ShouldError        bool
		ShouldVerify bool
		Signature *consensuspb.Signature
	}
	type testGenerator func(t *testing.T) (*testDescription)

	for _,testGen := range []testGenerator{
		func(t *testing.T) (*testDescription) {
			block := createBlock(t,nil)

			blockWithSig,err := consensus.OwnerSignBlock(block, aliceKey)
			assert.Nil(t, err, "setup valid block")

			sig := blockWithSig.Signatures[0]
			return &testDescription{
				Description: "valid everything",
				Block: blockWithSig,
				Signature: sig,
				ShouldError: false,
				ShouldVerify: true,
				InternalOwnership: &internalchain.InternalOwnership{
					PublicKeys: map[string]*consensuspb.PublicKey{
						aliceAddr.Hex(): {
							PublicKey: crypto.CompressPubkey(&aliceKey.PublicKey),
						},
					},
				},
			}
		},
	} {
		test := testGen(t)
		valid,err := consensus.VerifySignature(test.Block, test.InternalOwnership, test.Signature)

		if test.ShouldError {
			assert.NotNil(t, err, test.Description)
		} else {
			assert.Nil(t, err, test.Description)
		}

		if test.ShouldVerify {
			assert.True(t, valid, test.Description)
		} else {
			assert.False(t, valid, test.Description)
		}
	}
}

