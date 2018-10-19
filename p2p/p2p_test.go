package p2p

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func NewTestHost(ctx context.Context, t *testing.T) *Host {
	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	host, err := NewHost(ctx, key, GetRandomUnusedPort())

	require.Nil(t, err)
	require.NotNil(t, host)

	return host
}

func bootstrapAddresses(bootstrapHost *Host) []string {
	addresses := bootstrapHost.Addresses()
	return []string{addresses[len(addresses)-1].String()}
}

func TestBootstrap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bootstrap := NewTestHost(ctx, t)
	host := NewTestHost(ctx, t)
	_, err := host.Bootstrap(bootstrapAddresses(bootstrap))
	assert.Nil(t, err)
}
func TestSend(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bootstrap := NewTestHost(ctx, t)

	host := NewTestHost(ctx, t)
	_, err := host.Bootstrap(bootstrapAddresses(bootstrap))
	assert.Nil(t, err)

	host2 := NewTestHost(ctx, t)

	_, err = host2.Bootstrap(bootstrapAddresses(bootstrap))
	assert.Nil(t, err)

	msgs := make(chan []byte, 1)

	host2.SetHandler(func(data []byte) {
		msgs <- data
	})

	host.Send(host2.publicKey, []byte("hi"))

	received := <-msgs
	assert.Len(t, received, 2)
}
