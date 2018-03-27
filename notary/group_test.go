package notary_test

import (
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/quorumcontrol/qc3/notary"
	"testing"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/stretchr/testify/assert"
	"context"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/storage"
)

func TestVerifyNotaryGroupSignature(t *testing.T) {
	store := notary.NewChainStore("testTips", storage.NewMemStorage())

	privateKeys := make([]*bls.SignKey, 3)
	for i := 0; i < 3; i++ {
		key,err := bls.NewSignKey()
		assert.Nil(t, err)
		privateKeys[i] = key
	}

	publicKeys := make([]*consensuspb.PublicKey, len(privateKeys))
	for i,key := range privateKeys {
		verKey,_ := key.VerKey()
		publicKeys[i] = consensus.BlsKeyToPublicKey(verKey)
	}

	group := notary.GroupFromPublicKeys(publicKeys)

	signers := make([]*notary.Signer, len(privateKeys))
	for i,key := range privateKeys {
		signers[i] = notary.NewSigner(store, group, key)
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

			combined,err := defaultGroup.CombineSignatures([]*consensuspb.Signature{sig1,sig2}, nil)
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

			combined,err := defaultGroup.CombineSignatures([]*consensuspb.Signature{sig1}, nil)
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
		hsh,err := consensus.BlockToHash(test.Block)
		assert.Nil(t, err, test.Description)

		verified,err := test.Group.VerifySignature(hsh.Bytes(), test.Signature)
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

func TestGroup_CombineSignatures(t *testing.T) {
	privateKeys := make([]*bls.SignKey, 3)
	for i := 0; i < 3; i++ {
		key,err := bls.NewSignKey()
		assert.Nil(t, err)
		privateKeys[i] = key
	}

	publicKeys := make([]*consensuspb.PublicKey, len(privateKeys))
	for i,key := range privateKeys {
		verKey,_ := key.VerKey()
		publicKeys[i] = consensus.BlsKeyToPublicKey(verKey)
	}

	defaultGroup := notary.GroupFromPublicKeys(publicKeys)

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

			block,_ = consensus.BlsSignBlock(block, privateKeys[0])
			block,_ = consensus.BlsSignBlock(block, privateKeys[1])

			sig1 := block.Signatures[0]
			sig2 := block.Signatures[1]

			combined,err := defaultGroup.CombineSignatures([]*consensuspb.Signature{sig1,sig2}, nil)
			assert.Nil(t, err, "error setting up working sig")

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

			block,_ = consensus.BlsSignBlock(block, privateKeys[0])

			sig1 := block.Signatures[0]

			combined,err := defaultGroup.CombineSignatures([]*consensuspb.Signature{sig1}, nil)
			assert.Nil(t, err, "error setting up working sig")

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
		hsh,err := consensus.BlockToHash(test.Block)
		assert.Nil(t, err, test.Description)

		verified,err := test.Group.VerifySignature(hsh.Bytes(), test.Signature)
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

func TestGroup_ReplaceSignatures(t *testing.T) {
	privateKeys := make([]*bls.SignKey, 3)
	for i := 0; i < 3; i++ {
		key,err := bls.NewSignKey()
		assert.Nil(t, err)
		privateKeys[i] = key
	}

	publicKeys := make([]*consensuspb.PublicKey, len(privateKeys))
	for i,key := range privateKeys {
		verKey,_ := key.VerKey()
		publicKeys[i] = consensus.BlsKeyToPublicKey(verKey)
	}

	defaultGroup := notary.GroupFromPublicKeys(publicKeys)

	type testDescription struct {
		Description        string
		Block         *consensuspb.Block
		Group *notary.Group
		ShouldError        bool
		ShouldVerify bool
		TransactionsToVerify []*consensuspb.Transaction
	}
	type testGenerator func(t *testing.T) (*testDescription)

	for _,testGen := range []testGenerator{
		func(t *testing.T) (*testDescription) {
			block := createBlock(t, nil)

			block,_ = consensus.BlsSignBlock(block, privateKeys[0])
			block,_ = consensus.BlsSignBlock(block, privateKeys[1])

			return &testDescription{
				Description: "A block signed by 2/3 of the signers",
				Group: defaultGroup,
				Block: block,
				ShouldVerify: true,
			}
		},
		func(t *testing.T) (*testDescription) {
			block := createBlock(t, nil)
			transaction := consensus.EncapsulateTransaction(consensuspb.MINT_COIN, &consensuspb.MintCoinTransaction{
				Amount: 1,
				Name: block.SignableBlock.ChainId + ":catcoin",
			})
			block.SignableBlock.Transactions = []*consensuspb.Transaction{
				transaction,
			}
			block,_ = consensus.BlsSignTransaction(block, transaction, privateKeys[0])
			block,_ = consensus.BlsSignTransaction(block, transaction, privateKeys[1])

			block,_ = consensus.BlsSignBlock(block, privateKeys[0])
			block,_ = consensus.BlsSignBlock(block, privateKeys[1])

			return &testDescription{
				Description: "a block with signed transactions",
				Group: defaultGroup,
				Block: block,
				ShouldVerify: true,
				TransactionsToVerify: []*consensuspb.Transaction{transaction},
			}
		},
		func(t *testing.T) (*testDescription) {
			block := createBlock(t, nil)

			block,_ = consensus.BlsSignBlock(block, privateKeys[0])

			return &testDescription{
				Description: "A block only signed by 1/3 of the signers",
				Group: defaultGroup,
				Block: block,
				ShouldVerify: false,
				ShouldError: true,
			}
		},
	} {
		test := testGen(t)

		block,err := defaultGroup.ReplaceSignatures(test.Block)
		if test.ShouldError {
			assert.NotNil(t, err, test.Description)
		} else {
			assert.Nil(t,err,test.Description)

			verified,err := test.Group.IsBlockSigned(block)
			assert.Nil(t, err, "description: %v, err: %v", test.Description, err)

			if test.ShouldVerify {
				assert.True(t, verified, test.Description)
			} else {
				assert.False(t, verified, test.Description)
			}

			for _,transaction := range test.TransactionsToVerify {
				verified,err = test.Group.IsTransactionSigned(block, transaction)
				assert.Nil(t,err, test.Description)
				assert.True(t,verified, test.Description)
			}
		}
	}
}
