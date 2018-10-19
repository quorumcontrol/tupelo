package p2p

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewHost(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	host, err := NewHost(context.Background(), key, 10000)
	require.Nil(t, err)
	assert.NotNil(t, host)
}
