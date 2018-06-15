package notary_test

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/quorumcontrol/qc3/notary"
	"github.com/stretchr/testify/assert"
)

func TestIsNotGenesisOrIsValidGenesis(t *testing.T) {
	type testDescription struct {
		Description    string
		ChainTip       *consensuspb.ChainTip
		Block          *consensuspb.Block
		ShouldValidate bool
		ShouldError    bool
	}
	type testGenerator func(t *testing.T) *testDescription

	for _, testGen := range []testGenerator{
		func(t *testing.T) *testDescription {
			return &testDescription{
				Description:    "A totally blank chain and block",
				ChainTip:       &consensuspb.ChainTip{},
				Block:          &consensuspb.Block{},
				ShouldValidate: false,
			}
		},
		func(t *testing.T) *testDescription {
			block := createBlock(t, nil)
			signedBlock, err := consensus.OwnerSignBlock(block, aliceKey)
			assert.Nil(t, err, "setting up valid genesis")

			return &testDescription{
				Description:    "A valid genesis block",
				ChainTip:       &consensuspb.ChainTip{},
				Block:          signedBlock,
				ShouldValidate: true,
			}
		},
		func(t *testing.T) *testDescription {
			previousBlock := createBlock(t, nil)
			hsh, err := consensus.BlockToHash(previousBlock)
			assert.Nil(t, err, "setting up chain exists")

			existingChain := &consensuspb.ChainTip{
				LastHash: hsh.Bytes(),
				Authentication: &consensuspb.Authentication{
					PublicKeys: []*consensuspb.PublicKey{
						{
							Id:        aliceAddr.Hex(),
							PublicKey: crypto.CompressPubkey(&aliceKey.PublicKey),
						},
					},
				},
			}
			block := createBlock(t, previousBlock)
			signedBlock, err := consensus.OwnerSignBlock(block, bobKey)
			assert.Nil(t, err, "setting up valid signed")
			return &testDescription{
				Description:    "When a chain exists",
				ChainTip:       existingChain,
				Block:          signedBlock,
				ShouldValidate: true,
			}
		},
	} {
		test := testGen(t)
		res, err := notary.IsNotGenesisOrIsValidGenesis(context.Background(), test.ChainTip, test.Block)
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
