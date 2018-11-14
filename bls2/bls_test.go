package bls2

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
)

func TestSign(t *testing.T) {
	gen := NewGenerator()
	sk := NewSignKey()
	msg := []byte("hi")

	sig := Sign(msg, sk)

	assert.True(t, VerifySignature(sig, msg, newVerKey(gen, sk), gen))
	assert.False(t, VerifySignature(sig, []byte("other"), newVerKey(gen, sk), gen))
}

func TestMultiSignature(t *testing.T) {
	gen := NewGenerator()
	sk := NewSignKey()
	verKey1 := newVerKey(gen, sk)
	sk2 := NewSignKey()
	verKey2 := newVerKey(gen, sk2)

	msg := []byte("hi")

	sig1 := Sign(msg, sk)
	sig2 := Sign(msg, sk2)

	multiSig := NewMultiSignature([]*Signature{sig1, sig2})

	assert.True(t, VerifyMultiSignature(multiSig, msg, []*VerKey{verKey1, verKey2}, gen))
	assert.False(t, VerifyMultiSignature(multiSig, msg, []*VerKey{verKey1}, gen))
	assert.False(t, VerifyMultiSignature(multiSig, []byte("other"), []*VerKey{verKey1, verKey2}, gen))

}

func BenchmarkSignKey_Sign(b *testing.B) {
	msg := []byte("hi")
	hsh := crypto.Keccak256(msg)

	key := NewSignKey()

	var sig *Signature

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sig = Sign(hsh, key)
	}

	assert.NotNil(b, sig)
}

func BenchmarkVerKey_Verify(b *testing.B) {
	msg := []byte("hi")
	gen := NewGenerator()
	hsh := crypto.Keccak256(msg)

	key := NewSignKey()
	verKey := newVerKey(gen, key)

	sig := Sign(hsh, key)

	var isValid bool
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		isValid = VerifySignature(sig, hsh, verKey, gen)
	}

	assert.True(b, isValid)
}
