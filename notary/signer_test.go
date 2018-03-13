package notary_test

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/internalchain"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/quorumcontrol/qc3/consensus"
	"context"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/qc3/notary"
	"github.com/ethereum/go-ethereum/log"
)

func defaultNotary(t *testing.T) *notary.Signer {
	key,err := bls.NewSignKey()
	assert.Nil(t, err)

	storage := internalchain.NewMemStorage()

	return notary.NewSigner(storage, key)
}

func TestNewNotary(t *testing.T) {
	key,err := bls.NewSignKey()
	assert.Nil(t, err)

	storage := internalchain.NewMemStorage()

	notary := notary.NewSigner(storage, key)
	assert.Equal(t, notary.ChainStore, storage)
}

func TestNotary_CanSignBlock(t *testing.T) {
	type testDescription struct {
		Description        string
		Notary *notary.Signer
		Block         *consensuspb.Block
		ShouldValidate     bool
		ShouldError        bool
	}
	type testGenerator func(t *testing.T) (*testDescription)

	for _,testGen := range []testGenerator{
		func(t *testing.T) (*testDescription) {
			return &testDescription{
				Description: "A totally blank chain and block",
				Notary: defaultNotary(t),
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
				Notary: defaultNotary(t),
				Block: signedBlock,
				ShouldValidate: true,
			}
		},
		func(t *testing.T) (*testDescription) {
			previousBlock := createBlock(t,nil)
			existingChain := &consensuspb.Chain{
				Blocks: []*consensuspb.Block{previousBlock},
				Authentication: &consensuspb.Authentication{
					PublicKeys: []*consensuspb.PublicKey{
						{
							Id: aliceAddr.Hex(),
							PublicKey: crypto.CompressPubkey(&aliceKey.PublicKey),
						},
					},
				},
			}

			notary := defaultNotary(t)
			notary.ChainStore.Set(existingChain.Id, existingChain)

			block := createBlock(t, previousBlock)
			signedBlock,err := consensus.OwnerSignBlock(block, bobKey)
			assert.Nil(t, err, "setting up valid signed")
			return &testDescription{
				Description: "A new block on a stored chain, not signed by correct owners",
				Notary: notary,
				Block: signedBlock,
				ShouldValidate: false,
			}
		},
		func(t *testing.T) (*testDescription) {
			previousBlock := createBlock(t,nil)
			existingChain := &consensuspb.Chain{
				Id: consensus.AddrToDid(aliceAddr.Hex()),
				Blocks: []*consensuspb.Block{previousBlock},
				Authentication: &consensuspb.Authentication{
					PublicKeys: []*consensuspb.PublicKey{
						{
							Id: bobAddr.Hex(),
							PublicKey: crypto.CompressPubkey(&bobKey.PublicKey),
						},
					},
				},
			}

			notary := defaultNotary(t)
			notary.ChainStore.Set(existingChain.Id, existingChain)

			block := createBlock(t, previousBlock)
			signedBlock,err := consensus.OwnerSignBlock(block, bobKey)
			assert.Nil(t, err, "setting up valid signed")
			return &testDescription{
				Description: "A new block on a stored chain, signed by correct owners",
				Notary: notary,
				Block: signedBlock,
				ShouldValidate: true,
			}
		},
		func(t *testing.T) (*testDescription) {
			previousBlock := createBlock(t,nil)
			existingChain := &consensuspb.Chain{
				Id: consensus.AddrToDid(aliceAddr.Hex()),
				Blocks: []*consensuspb.Block{previousBlock},
				Authentication: &consensuspb.Authentication{
					PublicKeys: []*consensuspb.PublicKey{
						{
							Id: aliceAddr.Hex(),
							PublicKey: crypto.CompressPubkey(&aliceKey.PublicKey),
						},
					},
				},
				Authorizations: []*consensuspb.Authorization{
					{
						Type: consensuspb.UPDATE,
						Minimum: 2,
						Owners: []*consensuspb.Chain{
							chainFromEcdsaKey(t, &bobKey.PublicKey),
							chainFromEcdsaKey(t, &carolKey.PublicKey),
						},
					},
				},
			}

			notary := defaultNotary(t)
			notary.ChainStore.Set(existingChain.Id, existingChain)

			block := createBlock(t, previousBlock)
			signedBlock,err := consensus.OwnerSignBlock(block, carolKey)
			assert.Nil(t, err, "setting up valid signed")

			signedBlock,err = consensus.OwnerSignBlock(block, bobKey)
			assert.Nil(t, err, "setting up valid signed")

			return &testDescription{
				Description: "A new block on a stored chain, signed by a threshold of owners",
				Notary: notary,
				Block: signedBlock,
				ShouldValidate: true,
			}
		},
	} {
		test := testGen(t)
		didValidate,_,err := test.Notary.ValidateBlockLevel(context.Background(), test.Block)
		if test.ShouldError {
			assert.NotNil(t, err, err, test.Description)
		} else {
			assert.Nil(t, err, err, test.Description)
		}
		if test.ShouldValidate {
			assert.True(t, didValidate, test.Description)
		} else {
			assert.False(t, didValidate, test.Description)
		}
	}
}

func TestNotary_SignBlock(t *testing.T) {
	testNotary := defaultNotary(t)

	type testDescription struct {
		Description        string
		Block         *consensuspb.Block
		Notary *notary.Signer
		ShouldError        bool
	}
	type testGenerator func(t *testing.T) (*testDescription)

	for _,testGen := range []testGenerator{
		func(t *testing.T) (*testDescription) {
			return &testDescription{
				Description: "valid everything",
				Block:       createBlock(t, nil),
				Notary:  testNotary,
				ShouldError: false,
			}
		},
	} {
		test := testGen(t)
		sigLength := len(test.Block.Signatures)
		blockWithSig,err := test.Notary.SignBlock(context.Background(), test.Block)
		if test.ShouldError {
			assert.NotNil(t, err, test.Description)
		} else {
			assert.Nil(t, err, test.Description)
		}
		assert.Equal(t, len(blockWithSig.Signatures), sigLength + 1, test.Description)
		assert.Equal(t, blockWithSig.Signatures[len(blockWithSig.Signatures) - 1].Creator, testNotary.Id())
	}
}

func TestNotary_ProcessBlock(t *testing.T) {
	//log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(log.LvlTrace), log.StreamHandler(os.Stderr, log.TerminalFormat(false))))

	type testDescription struct {
		Description        string
		Block         *consensuspb.Block
		Notary *notary.Signer
		Validator        func(t *testing.T, chain *consensuspb.Chain, block *consensuspb.Block, err error)
	}
	type testGenerator func(t *testing.T) (*testDescription)

	for _,testGen := range []testGenerator{
		func(t *testing.T) (*testDescription) {
			testNotary := defaultNotary(t)
			block := createBlock(t, nil)
			signedBlock,err := consensus.OwnerSignBlock(block, aliceKey)
			assert.Nil(t,err, "setting up just add data %v", err)

			return &testDescription{
				Description: "block with just add data",
				Block: signedBlock,
				Notary:  testNotary,
				Validator: func(t *testing.T, savedChain *consensuspb.Chain, block *consensuspb.Block, err error) {
					assert.Nil(t, err)
					assert.NotNil(t,block)
					assert.Equal(t, block, savedChain.Blocks[0])
				},
			}
		},
		func(t *testing.T) (*testDescription) {
			testNotary := defaultNotary(t)
			block := createBlock(t, nil)

			updateTrans := &consensuspb.UpdateOwnershipTransaction{
				ChainId: block.SignableBlock.ChainId,
				Authentication: &consensuspb.Authentication{
					PublicKeys: []*consensuspb.PublicKey{
						{
							Id: bobAddr.Hex(),
							PublicKey: crypto.CompressPubkey(&bobKey.PublicKey),
						},
					},
				},
			}
			trans := consensus.EncapsulateTransaction(consensuspb.UPDATE_OWNERSHIP, updateTrans)

			block.SignableBlock.Transactions = append(block.SignableBlock.Transactions, trans)
			signedBlock,err := consensus.OwnerSignBlock(block, aliceKey)
			assert.Nil(t,err, "setting up with ownership change %v", err)

			return &testDescription{
				Description: "block with just add data",
				Block: signedBlock,
				Notary:  testNotary,
				Validator: func(t *testing.T, savedChain *consensuspb.Chain, block *consensuspb.Block, err error) {
					assert.Nil(t, err)
					assert.NotNil(t,block)
					assert.Equal(t, block, savedChain.Blocks[0])
					assert.Equal(t, savedChain.Authentication.PublicKeys[0].Id, updateTrans.Authentication.PublicKeys[0].Id)
				},
			}
		},
	} {
		test := testGen(t)
		processed, err := test.Notary.ProcessBlock(context.Background(), test.Block)
		log.Trace("processed", "processed", processed)
		chain,chnerr := test.Notary.ChainStore.Get(test.Block.SignableBlock.ChainId)
		assert.Nil(t, chnerr, test.Description)

		test.Validator(t, chain, processed, err)
	}
}
