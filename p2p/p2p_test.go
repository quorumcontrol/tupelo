package p2p

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func LibP2PNodeGenerator(ctx context.Context, t *testing.T) Node {
	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	host, err := NewLibP2PHost(ctx, key, 0)

	require.Nil(t, err)
	require.NotNil(t, host)

	return host
}

func TestHost(t *testing.T) {
	NodeTests(t, LibP2PNodeGenerator)
}
