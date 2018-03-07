package bls

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/ethereum/go-ethereum/common"
)

func TestNewSignKey(t *testing.T) {
	key,err := NewSignKey()
	assert.Nil(t, err)
	assert.Len(t, key.Bytes(), 32)
}

func TestSignKey_Sign(t *testing.T) {
	msg := []byte("hi")
	hsh := common.BytesToHash(msg).Bytes()

	key,err := NewSignKey()
	assert.Nil(t, err)

	sig,err := key.Sign(hsh)
	assert.Nil(t, err)
	assert.Len(t, sig, 128)
}

func TestNewGenerator(t *testing.T) {
	gen,err := NewGenerator()
	// uncomment to create a new generator
	//t.Logf("a generator: '%s'", hexutil.Encode(gen))
	assert.Nil(t, err)
	assert.Len(t, gen, 128)
}

func TestSignKey_VerKey(t *testing.T) {
	key,err := NewSignKey()
	assert.Nil(t, err)

	verKey, err := key.VerKey()
	assert.Nil(t, err)

	assert.Len(t, verKey.Bytes(), 128)
}

func TestCSignatureFrom(t *testing.T) {
	msg := []byte("hi")
	hsh := common.BytesToHash(msg).Bytes()

	key,err := NewSignKey()
	assert.Nil(t, err)

	sig,err := key.Sign(hsh)
	assert.Nil(t, err)

	_,err = cSignatureFrom(sig)
	assert.Nil(t,err)
}

func TestVerKey_Verify(t *testing.T) {
	msg := []byte("hi")
	hsh := common.BytesToHash(msg).Bytes()

	key,err := NewSignKey()
	assert.Nil(t, err)

	sig,err := key.Sign(hsh)
	assert.Nil(t, err)

	verKey,err := key.VerKey()
	assert.Nil(t, err)

	isValid,err := verKey.Verify(sig, hsh)
	assert.Nil(t, err)
	assert.True(t, isValid)

}
