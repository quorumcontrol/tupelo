package bls

import (
	"strconv"
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

func TestSumPublics(t *testing.T) {
	key1, err := NewSignKey()
	assert.Nil(t, err)

	key2, err := NewSignKey()
	assert.Nil(t, err)

	aggregate, err := SumPublics([][]byte{key1.MustVerKey().Bytes(), key2.MustVerKey().Bytes()})
	require.Nil(t, err)
	assert.IsType(t, &VerKey{}, aggregate)
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

func TestBatchVerify(t *testing.T) {
	sigCount := 5
	msgs := make([][]byte, sigCount)
	sigs := make([][]byte, sigCount)
	verKeys := make([]*VerKey, sigCount)
	for i := 0; i < sigCount; i++ {
		msgs[i] = []byte("hi" + strconv.Itoa(i))
		key, err := NewSignKey()
		assert.Nil(t, err)
		verKeys[i] = key.MustVerKey()
		sig, err := key.Sign(msgs[i])
		require.Nil(t, err)
		sigs[i] = sig
	}
	// in the valid case
	valid, err := BatchVerify(msgs, verKeys, sigs)
	require.Nil(t, err)
	assert.True(t, valid)

	// with one bad sig
	msgs[0][0] ^= 0x01
	valid, err = BatchVerify(msgs, verKeys, sigs)
	require.Nil(t, err)
	assert.False(t, valid)
}

func BenchmarkBatchVerify(b *testing.B) {
	sigCount := 100
	msgs := make([][]byte, sigCount)
	sigs := make([][]byte, sigCount)
	verKeys := make([]*VerKey, sigCount)
	for i := 0; i < sigCount; i++ {
		msgs[i] = []byte("hi" + strconv.Itoa(i))
		key, err := NewSignKey()
		assert.Nil(b, err)
		verKeys[i] = key.MustVerKey()
		sig, err := key.Sign(msgs[i])
		require.Nil(b, err)
		sigs[i] = sig
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		isValid, _ := BatchVerify(msgs, verKeys, sigs)
		assert.True(b, isValid)
	}
}

func BenchmarkVerifyMultiSig(b *testing.B) {
	sigCount := 100
	sigs := make([][]byte, sigCount)
	verKeys := make([][]byte, sigCount)
	msg := []byte("hi")

	for i := 0; i < sigCount; i++ {
		key, err := NewSignKey()
		assert.Nil(b, err)
		verKeys[i] = key.MustVerKey().Bytes()
		sig, err := key.Sign(msg)
		require.Nil(b, err)
		sigs[i] = sig
	}
	summed, err := SumSignatures(sigs)
	require.Nil(b, err)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		isValid, _ := VerifyMultiSig(summed, msg, verKeys)
		assert.True(b, isValid)
	}
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
