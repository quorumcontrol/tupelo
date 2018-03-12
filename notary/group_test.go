package notary_test

import (
	"github.com/quorumcontrol/qc3/internalchain"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/quorumcontrol/qc3/notary"
	"testing"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/stretchr/testify/assert"
	"context"
)

func TestVerifyNotaryGroupSignature(t *testing.T) {
	store := internalchain.NewMemStorage()

	privateKeys := make([]*bls.SignKey, 3)
	for i := 0; i < 3; i++ {
		key,err := bls.NewSignKey()
		assert.Nil(t, err)
		privateKeys[i] = key
	}

	publicKeys := make([]*consensuspb.PublicKey, len(privateKeys))
	for i,key := range privateKeys {
		verKey,_ := key.VerKey()
		publicKeys[i] = &consensuspb.PublicKey{
			PublicKey: verKey.Bytes(),
			Type: consensuspb.BLSGroupSig,
			Id: notary.PublicKeyToAddress(verKey.Bytes()).Hex(),
		}
	}

	signers := make([]*notary.Signer, len(privateKeys))
	for i,key := range privateKeys {
		signers[i] = notary.NewSigner(store, key)
	}

	defaultGroup := notary.NewGroup("testTest", publicKeys)

	type testDescription struct {
		Description        string
		Block         *consensuspb.Block
		Group *notary.Group
		ShouldError        bool
		ShouldVerify bool
		Signature *consensuspb.Signature
	}
	type testGenerator func(t *testing.T) (*testDescription)

	for _,testGen := range []testGenerator{
		func(t *testing.T) (*testDescription) {
			block := createBlock(t, nil)

			block,_ = signers[0].SignBlock(context.Background(), block)
			block,_ = signers[1].SignBlock(context.Background(), block)

			sig1 := block.Signatures[0]
			sig2 := block.Signatures[1]

			combined,err := defaultGroup.CombineSignatures([]*consensuspb.Signature{sig1,sig2})
			assert.Nil(t, err, "error setting up working sig")

			block.Signatures = append(block.Signatures, combined)

			return &testDescription{
				Description: "A block signed by 2/3 of the signers",
				Group: defaultGroup,
				Block: block,
				ShouldVerify: true,
				Signature: combined,
			}
		},
		func(t *testing.T) (*testDescription) {
			block := createBlock(t, nil)

			block,_ = signers[0].SignBlock(context.Background(), block)

			sig1 := block.Signatures[0]

			combined,err := defaultGroup.CombineSignatures([]*consensuspb.Signature{sig1})
			assert.Nil(t, err, "error setting up working sig")

			block.Signatures = append(block.Signatures, combined)

			return &testDescription{
				Description: "A block only signed by 1/3 of the signers",
				Group: defaultGroup,
				Block: block,
				ShouldVerify: false,
				Signature: combined,
			}
		},
	} {
		test := testGen(t)
		verified,err := test.Group.VerifySignature(test.Block, test.Signature)
		if test.ShouldError {
			assert.NotNil(t, err, test.Description)
		} else {
			assert.Nil(t,err,test.Description)
		}

		if test.ShouldVerify {
			assert.True(t, verified, test.Description)
		} else {
			assert.False(t, verified, test.Description)
		}
	}
}
