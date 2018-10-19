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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	host, err := NewHost(ctx, key, 10000)

	require.Nil(t, err)
	assert.NotNil(t, host)
}

func TestBootstrap(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	host, err := NewHost(ctx, key, 10000)

	require.Nil(t, err)
	assert.NotNil(t, host)

	_, err = host.Bootstrap(LocalBootstrap)
	assert.Nil(t, err)
}
