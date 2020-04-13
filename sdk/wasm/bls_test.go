// +build wasm

package main

import (
	"testing"

	"github.com/quorumcontrol/tupelo/sdk/bls"
	"github.com/stretchr/testify/assert"
)

func TestVerKey_Verify(t *testing.T) {
	msg := []byte("hi")

	key, err := bls.NewSignKey()
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
