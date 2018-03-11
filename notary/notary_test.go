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
			existingChain := &internalchain.InternalChain{
				Id: consensus.AddrToDid(aliceAddr.Hex()),
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
			existingChain := &internalchain.InternalChain{
				Id: consensus.AddrToDid(aliceAddr.Hex()),
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
			existingChain := &internalchain.InternalChain{
				Id: consensus.AddrToDid(aliceAddr.Hex()),
				LastBlock: previousBlock,
				MinimumOwners: 2,
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
					{
						Name: consensus.AddrToDid(bobAddr.Hex()),
						PublicKeys: map[string]*consensuspb.PublicKey{
							bobAddr.Hex(): {
								Id: bobAddr.Hex(),
								PublicKey: crypto.CompressPubkey(&bobKey.PublicKey),
							},
						},
					},{
						Name: consensus.AddrToDid(carolAddr.Hex()),
						PublicKeys: map[string]*consensuspb.PublicKey{
							carolAddr.Hex(): {
								Id: carolAddr.Hex(),
								PublicKey: crypto.CompressPubkey(&carolKey.PublicKey),
							},
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
		res,err := test.Notary.CanSignBlock(context.Background(), test.Block)
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
		assert.Equal(t, blockWithSig.Signatures[len(blockWithSig.Signatures) - 1].Creator, testNotary.NodeId())
	}

}