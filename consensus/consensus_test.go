package consensus

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/tupelo/bls"
	"github.com/stretchr/testify/assert"
)

func TestIsBlockSignedBy(t *testing.T) {
	key, err := crypto.GenerateKey()
	assert.Nil(t, err)

	blockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]string{
						"path":  "down/in/the/thing",
						"value": "hi",
					},
				},
			},
		},
	}

	signed, err := SignBlock(blockWithHeaders, key)

	assert.Nil(t, err)

	isSigned, err := IsBlockSignedBy(signed, crypto.PubkeyToAddress(key.PublicKey).String())

	assert.Nil(t, err)
	assert.True(t, isSigned)

}

func TestVerify(t *testing.T) {
	type testCase struct {
		Description   string
		PublicKey     PublicKey
		Payload       []byte
		Signature     Signature
		ShouldSucceed bool
		ShouldError   bool
	}
	for _, testCreator := range []func() testCase{
		func() testCase {
			key, err := crypto.GenerateKey()
			assert.Nil(t, err)

			return testCase{
				Description:   "blank payload and signature",
				PublicKey:     EcdsaToPublicKey(&key.PublicKey),
				ShouldSucceed: false,
				ShouldError:   true,
			}
		},

		func() testCase {
			key, err := crypto.GenerateKey()
			assert.Nil(t, err)

			payload := []byte("hi")

			sig, err := EcdsaSign(payload, key)
			assert.Nil(t, err)

			return testCase{
				Description:   "good ecdsa signature",
				PublicKey:     EcdsaToPublicKey(&key.PublicKey),
				Payload:       payload,
				Signature:     *sig,
				ShouldSucceed: true,
				ShouldError:   false,
			}
		},

		func() testCase {
			key, err := crypto.GenerateKey()
			assert.Nil(t, err)

			payload := []byte("hi")

			sig, err := EcdsaSign([]byte("different payload"), key)
			assert.Nil(t, err)

			return testCase{
				Description:   "bad ecdsa signature",
				PublicKey:     EcdsaToPublicKey(&key.PublicKey),
				Payload:       payload,
				Signature:     *sig,
				ShouldSucceed: false,
				ShouldError:   false,
			}
		},

		func() testCase {
			key, err := bls.NewSignKey()
			assert.Nil(t, err)

			payload := crypto.Keccak256([]byte("hi"))

			sig, err := BlsSign(payload, key)
			assert.Nil(t, err)

			return testCase{
				Description:   "good bls signature",
				PublicKey:     BlsKeyToPublicKey(key.MustVerKey()),
				Payload:       payload,
				Signature:     *sig,
				ShouldSucceed: true,
				ShouldError:   false,
			}
		},

		func() testCase {
			key, err := bls.NewSignKey()
			assert.Nil(t, err)

			payload := crypto.Keccak256([]byte("hi"))

			sig, err := BlsSign([]byte("different payload"), key)
			assert.Nil(t, err)

			return testCase{
				Description:   "bad bls signature",
				PublicKey:     BlsKeyToPublicKey(key.MustVerKey()),
				Payload:       payload,
				Signature:     *sig,
				ShouldSucceed: false,
				ShouldError:   false,
			}
		},
	} {
		test := testCreator()

		hsh, err := ObjToHash(test.Payload)
		assert.Nil(t, err)

		isVerified, err := Verify(hsh, test.Signature, test.PublicKey)
		if test.ShouldError {
			assert.NotNil(t, err)
		} else {
			assert.Nil(t, err)
		}

		if test.ShouldSucceed {
			assert.True(t, isVerified, test.Description)
		} else {
			assert.False(t, isVerified, test.Description)
		}
	}
}
