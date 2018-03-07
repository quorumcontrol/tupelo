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
	hsh := common.BytesToHash(msg)

	key,err := NewSignKey()
	assert.Nil(t, err)

	sig,err := key.Sign(hsh.Bytes())
	assert.Nil(t, err)
	assert.Len(t, sig, 32)
}

