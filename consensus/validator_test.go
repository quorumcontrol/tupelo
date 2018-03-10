package consensus_test

import (
	"testing"
	"github.com/quorumcontrol/qc3/internalchain"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/stretchr/testify/assert"
	"context"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestIsValidOwnerSig(t *testing.T) {
	type testDescription struct {
		Description        string
		InternalChain *internalchain.InternalChain
		Block         *consensuspb.Block
		ShouldValidate     bool
		ShouldError        bool
	}
	type testGenerator func(t *testing.T) (*testDescription)

	for _,testGen := range []testGenerator{
		func(t *testing.T) (*testDescription) {
			return &testDescription{
				Description: "A totally blank chain and block",
				InternalChain: &internalchain.InternalChain{},
				Block: &consensuspb.Block{},
				ShouldValidate: false,
			}
		},
		func(t *testing.T) (*testDescription) {
			block := createBlock(t,nil)
			signedBlock,err := consensus.OwnerSignBlock(block, aliceKey)
			assert.Nil(t, err, "setting up valid genesis")

			return &testDescription{
				Description: "A valid genesis block",
				InternalChain: &internalchain.InternalChain{},
				Block: signedBlock,
				ShouldValidate: true,
			}
		},
		func(t *testing.T) (*testDescription) {
			previousBlock := createBlock(t,nil)
			existingChain := &internalchain.InternalChain{
				LastBlock: previousBlock,
				MinimumOwners: 1,
				CurrentOwners: []*internalchain.InternalOwnership{
					{
						Name: consensus.AddrToDid(aliceAddr.Hex()),
						PublicKeys: map[string]*consensuspb.PublicKey{
							aliceAddr.Hex(): {
								Id: aliceAddr.Hex(),
								PublicKey: crypto.CompressPubkey(&aliceKey.PublicKey),
							},
						},
					},
				},
			}
			block := createBlock(t, previousBlock)
			signedBlock,err := consensus.OwnerSignBlock(block, bobKey)
			assert.Nil(t, err, "setting up valid signed")
			return &testDescription{
				Description: "A new block on a stored chain, not signed by correct owners",
				InternalChain: existingChain,
				Block: signedBlock,
				ShouldValidate: false,
			}
		},
		func(t *testing.T) (*testDescription) {
			previousBlock := createBlock(t,nil)
			existingChain := &internalchain.InternalChain{
				LastBlock: previousBlock,
				MinimumOwners: 1,
				CurrentOwners: []*internalchain.InternalOwnership{
					{
						Name: consensus.AddrToDid(bobAddr.Hex()),
						PublicKeys: map[string]*consensuspb.PublicKey{
							bobAddr.Hex(): {
								Id: bobAddr.Hex(),
								PublicKey: crypto.CompressPubkey(&bobKey.PublicKey),
							},
						},
					},
				},
			}
			block := createBlock(t, previousBlock)
			signedBlock,err := consensus.OwnerSignBlock(block, bobKey)
			assert.Nil(t, err, "setting up valid signed")
			return &testDescription{
				Description: "A new block on a stored chain, signed by correct owners",
				InternalChain: existingChain,
				Block: signedBlock,
				ShouldValidate: true,
			}
		},
		} {
		test := testGen(t)
		res,err := consensus.IsValidOwnerSig(context.Background(), test.InternalChain, test.Block)
		if test.ShouldError {
			assert.NotNil(t, err, err, test.Description)
		} else {
			assert.Nil(t, err, err, test.Description)
		}
		if test.ShouldValidate {
			assert.True(t, res, test.Description)
		} else {
			assert.False(t, res, test.Description)
		}
	}
}
