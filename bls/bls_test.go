package bls

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
)

func TestNewSignKey(t *testing.T) {
	key, err := NewSignKey()
	assert.Nil(t, err)
	assert.Len(t, key.Bytes(), 32)
}

func TestSignKey_Sign(t *testing.T) {
	msg := []byte("hi")
	hsh := crypto.Keccak256(msg)

	key, err := NewSignKey()
	assert.Nil(t, err)

	SetupLogger()

	sig, err := key.Sign(hsh)
	assert.Nil(t, err)
	assert.Len(t, sig, 128)
}

func TestNewGenerator(t *testing.T) {
	gen, err := NewGenerator()
	// uncomment to create a new generator
	//t.Logf("a generator: '%s'", hexutil.Encode(gen))
	assert.Nil(t, err)
	assert.Len(t, gen, 128)
}

func TestSignKey_VerKey(t *testing.T) {
	key, err := NewSignKey()
	assert.Nil(t, err)

	verKey, err := key.VerKey()
	assert.Nil(t, err)

	assert.Len(t, verKey.Bytes(), 128)
}

func TestCSignatureFrom(t *testing.T) {
	msg := []byte("hi")
	hsh := crypto.Keccak256(msg)

	key, err := NewSignKey()
	assert.Nil(t, err)

	sig, err := key.Sign(hsh)
	assert.Nil(t, err)

	_, err = cSignatureFrom(sig)
	assert.Nil(t, err)
}

func TestVerKey_Verify(t *testing.T) {
	msg := []byte("hi")
	hsh := crypto.Keccak256(msg)

	key, err := NewSignKey()
	assert.Nil(t, err)

	sig, err := key.Sign(hsh)
	assert.Nil(t, err)

	verKey, err := key.VerKey()
	assert.Nil(t, err)

	isValid, err := verKey.Verify(sig, hsh)
	assert.Nil(t, err)
	assert.True(t, isValid)

	// with invalid message
	invalidRes, err := verKey.Verify(sig, crypto.Keccak256([]byte("abc")))
	assert.Nil(t, err)
	assert.False(t, invalidRes)
}

func TestSumSignatures(t *testing.T) {
	msg := []byte("hi")
	hsh := crypto.Keccak256(msg)

	key1, err := NewSignKey()
	assert.Nil(t, err)

	key2, err := NewSignKey()
	assert.Nil(t, err)

	sig1, err := key1.Sign(hsh)
	assert.Nil(t, err)

	sig2, err := key2.Sign(hsh)
	assert.Nil(t, err)

	multiSig, err := SumSignatures([][]byte{sig1, sig2})
	assert.Nil(t, err)
	assert.Len(t, multiSig, 128)
}

func TestVerifyMultiSig(t *testing.T) {
	msg := []byte("hi")
	hsh := crypto.Keccak256(msg)

	key1, err := NewSignKey()
	assert.Nil(t, err)

	key2, err := NewSignKey()
	assert.Nil(t, err)

	key3, err := NewSignKey()
	assert.Nil(t, err)

	sig1, err := key1.Sign(hsh)
	assert.Nil(t, err)

	sig2, err := key2.Sign(hsh)
	assert.Nil(t, err)

	multiSig, err := SumSignatures([][]byte{sig1, sig2})
	assert.Nil(t, err)
	assert.Len(t, multiSig, 128)

	verKeys := make([][]byte, 2)
	verKey1, err := key1.VerKey()
	assert.Nil(t, err)
	verKey2, err := key2.VerKey()
	assert.Nil(t, err)
	verKey3, err := key3.VerKey()

	verKeys[0] = verKey1.Bytes()
	verKeys[1] = verKey2.Bytes()

	isValid, err := VerifyMultiSig(multiSig, hsh, verKeys)
	assert.Nil(t, err)
	assert.True(t, isValid)

	// with invalid message
	invalidRes, err := VerifyMultiSig(multiSig, crypto.Keccak256([]byte("abc")), verKeys)
	assert.Nil(t, err)
	assert.False(t, invalidRes)

	// with missing key
	verKeys = make([][]byte, 3)

	verKeys[0] = verKey1.Bytes()
	verKeys[1] = verKey2.Bytes()
	verKeys[2] = verKey3.Bytes()

	invalidRes, err = VerifyMultiSig(multiSig, hsh, verKeys)
	assert.Nil(t, err)
	assert.False(t, invalidRes)

	// with a single verkey

	multiSig, err = SumSignatures([][]byte{sig1})
	assert.Nil(t, err)
	assert.Len(t, multiSig, 128)

	verKeys = make([][]byte, 1)
	verKeys[0] = verKey1.Bytes()

	isValid, err = VerifyMultiSig(multiSig, hsh, verKeys)
	assert.Nil(t, err)
	assert.True(t, isValid)
}
