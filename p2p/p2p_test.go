package p2p

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func NewTestHost(ctx context.Context, t *testing.T) *Host {
	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	host, err := NewHost(ctx, key, 0)

	require.Nil(t, err)
	require.NotNil(t, host)

	return host
}

func bootstrapAddresses(bootstrapHost *Host) []string {
	addresses := bootstrapHost.Addresses()
	for _, addr := range addresses {
		addrStr := addr.String()
		if strings.Contains(addrStr, "127.0.0.1") {
			return []string{addrStr}
		}
	}
	return nil
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

	host2.SetStreamHandler("test/protocol", func(s net.Stream) {
		data, err := ioutil.ReadAll(s)
		if err != nil {
			fmt.Printf("error reading: %v", err)
		}
		s.Close()
		msgs <- data
	})

	host.Send(host2.publicKey, "test/protocol", []byte("hi"))

	received := <-msgs
	assert.Len(t, received, 2)
}
