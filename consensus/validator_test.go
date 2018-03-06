package consensus_test

import (
	"testing"
	"github.com/quorumcontrol/qc3/internalchain"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/stretchr/testify/assert"
)

func TestValidGenesisBlock(t *testing.T) {
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

			block := genValidGenesisBlock(t)
			signedBlock,err := consensus.OwnerSignBlock(block, aliceKey)
			assert.Nil(t, err, "setting up valid genesis")

			return &testDescription{
				Description: "A valid genesis block",
				InternalChain: &internalchain.InternalChain{},
				Block: signedBlock,
				ShouldValidate: true,
			}
		},
	} {
		test := testGen(t)
		res,err := consensus.IsValidGenesisBlock(test.InternalChain, test.Block)
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
