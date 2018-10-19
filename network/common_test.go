package network

import (
	"context"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/qc3/p2p"
	"github.com/stretchr/testify/require"
)

func newBootstrapHost(ctx context.Context, t *testing.T) *p2p.Host {
	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	host, err := p2p.NewHost(ctx, key, p2p.GetRandomUnusedPort())

	require.Nil(t, err)
	require.NotNil(t, host)

	return host
}

func bootstrapAddresses(bootstrapHost *p2p.Host) []string {
	addresses := bootstrapHost.Addresses()
	for _, addr := range addresses {
		addrStr := addr.String()
		if strings.Contains(addrStr, "127.0.0.1") {
			return []string{addrStr}
		}
	}
	return nil
}
