package bls

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewSignKey(t *testing.T) {
	key, err := NewSignKey()
	assert.Nil(t, err)
	assert.Len(t, key.Bytes(), 32)
}

func TestSignKey_Sign(t *testing.T) {
	msg := []byte("hi")

	key, err := NewSignKey()
	assert.Nil(t, err)

	sig, err := key.Sign(msg)
	assert.Nil(t, err)
	assert.Len(t, sig, 64)
}

func TestSignKey_VerKey(t *testing.T) {
	key, err := NewSignKey()
	assert.Nil(t, err)

	verKey, err := key.VerKey()
	assert.Nil(t, err)

	assert.Len(t, verKey.Bytes(), 128)
}

func TestVerKey_Verify(t *testing.T) {
	msg := []byte("hi")

	key, err := NewSignKey()
	assert.Nil(t, err)

	sig, err := key.Sign(msg)
	assert.Nil(t, err)

	verKey, err := key.VerKey()
	assert.Nil(t, err)

	isValid, err := verKey.Verify(sig, msg)
	assert.Nil(t, err)
	assert.True(t, isValid)

	// with invalid message
	invalidRes, err := verKey.Verify(sig, []byte("invalidmessage"))
	assert.Nil(t, err)
	assert.False(t, invalidRes)
}

func TestSumSignatures(t *testing.T) {
	msg := []byte("hi")

	key1, err := NewSignKey()
	assert.Nil(t, err)

	key2, err := NewSignKey()
	assert.Nil(t, err)

	sig1, err := key1.Sign(msg)
	assert.Nil(t, err)

	sig2, err := key2.Sign(msg)
	assert.Nil(t, err)

	multiSig, err := SumSignatures([][]byte{sig1, sig2})
	assert.Nil(t, err)
	assert.Len(t, multiSig, 64)
}

func TestVerifyMultiSig(t *testing.T) {
	msg := []byte("hi")

	key1, err := NewSignKey()
	assert.Nil(t, err)

	key2, err := NewSignKey()
	assert.Nil(t, err)

	key3, err := NewSignKey()
	assert.Nil(t, err)

	sig1, err := key1.Sign(msg)
	assert.Nil(t, err)

	sig2, err := key2.Sign(msg)
	assert.Nil(t, err)

	multiSig, err := SumSignatures([][]byte{sig1, sig2})
	assert.Nil(t, err)
	assert.Len(t, multiSig, 64)

	verKeys := make([][]byte, 2)
	verKey1, err := key1.VerKey()
	assert.Nil(t, err)
	verKey2, err := key2.VerKey()
	assert.Nil(t, err)
	verKey3, err := key3.VerKey()
	assert.Nil(t, err)

	verKeys[0] = verKey1.Bytes()
	verKeys[1] = verKey2.Bytes()

	isValid, err := VerifyMultiSig(multiSig, msg, verKeys)
	assert.Nil(t, err)
	assert.True(t, isValid)

	// with invalid message
	invalidRes, err := VerifyMultiSig(multiSig, []byte("invalidmessage"), verKeys)
	assert.Nil(t, err)
	assert.False(t, invalidRes)

	// with missing key
	verKeys = make([][]byte, 3)

	verKeys[0] = verKey1.Bytes()
	verKeys[1] = verKey2.Bytes()
	verKeys[2] = verKey3.Bytes()

	invalidRes, err = VerifyMultiSig(multiSig, msg, verKeys)
	assert.Nil(t, err)
	assert.False(t, invalidRes)

	// with a single verkey

	multiSig, err = SumSignatures([][]byte{sig1})
	assert.Nil(t, err)
	assert.Len(t, multiSig, 64)

	verKeys = make([][]byte, 1)
	verKeys[0] = verKey1.Bytes()

	isValid, err = VerifyMultiSig(multiSig, msg, verKeys)
	assert.Nil(t, err)
	assert.True(t, isValid)
}

func TestSumVerKeys(t *testing.T) {
	msg := []byte("hi")

	key1, err := NewSignKey()
	assert.Nil(t, err)

	key2, err := NewSignKey()
	assert.Nil(t, err)

	sig1, err := key1.Sign(msg)
	assert.Nil(t, err)

	sig2, err := key2.Sign(msg)
	assert.Nil(t, err)

	multiSig, err := SumSignatures([][]byte{sig1, sig1, sig2})
	assert.Nil(t, err)
	assert.Len(t, multiSig, 64)

	aggregateKey, err := SumVerKeys([]*VerKey{key1.MustVerKey(), key1.MustVerKey(), key2.MustVerKey()})
	require.Nil(t, err)
	require.NotNil(t, aggregateKey)

	valid, err := aggregateKey.Verify(multiSig, msg)
	require.Nil(t, err)
	require.True(t, valid)

	// test roundtrip to bits ( makes sure this doesn't affect: https://github.com/dedis/kyber/issues/400 )
	bits := aggregateKey.Bytes()
	require.Nil(t, err)
	restoredKey := BytesToVerKey(bits)
	require.NotNil(t, restoredKey)

}

func BenchmarkVerKey_Verify(b *testing.B) {
	msg := []byte("hi")

	key, err := NewSignKey()
	require.Nil(b, err)

	verKey := key.MustVerKey()

	sig, err := key.Sign(msg)
	require.Nil(b, err)

	var isValid bool

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v, _ := verKey.Verify(sig, msg)
		isValid = v
	}

	assert.True(b, isValid)
}

func BenchmarkSignKey_Sign(b *testing.B) {
	msg := []byte("hi")

	key, err := NewSignKey()
	assert.Nil(b, err)

	var sig []byte

	for i := 0; i < b.N; i++ {
		sig, _ = key.Sign(msg)
	}

	assert.Len(b, sig, 64)
}
