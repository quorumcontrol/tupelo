package p2p

import (
	"context"
	"io/ioutil"
	"strings"
	"testing"

	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nodeGenerator func(ctx context.Context, t *testing.T) Node

func NodeTests(t *testing.T, generator nodeGenerator) {
	BootstrapTest(t, generator)
	SendTest(t, generator)
}

func bootstrapAddresses(bootstrapHost Node) []string {
	addresses := bootstrapHost.Addresses()
	for _, addr := range addresses {
		addrStr := addr.String()
		if strings.Contains(addrStr, "127.0.0.1") {
			return []string{addrStr}
		}
	}
	return nil
}

func BootstrapTest(t *testing.T, generator nodeGenerator) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	nodeA := generator(ctx, t)
	nodeB := generator(ctx, t)

	_, err := nodeA.Bootstrap(bootstrapAddresses(nodeB))
	assert.Nil(t, err)
}

func SendTest(t *testing.T, generator nodeGenerator) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	nodeA := generator(ctx, t)
	nodeB := generator(ctx, t)

	_, err := nodeA.Bootstrap(bootstrapAddresses(nodeB))
	assert.Nil(t, err)

	msgs := make(chan []byte, 1)

	nodeB.SetStreamHandler("test/protocol", func(s net.Stream) {
		defer s.Close()
		data, err := ioutil.ReadAll(s)
		require.Nil(t, err)
		msgs <- data
	})

	nodeA.Send(nodeB.PublicKey(), "test/protocol", []byte("hi"))

	received := <-msgs
	assert.Len(t, received, 2)
}

func SendAndReceiveTest(t *testing.T, generator nodeGenerator) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	nodeA := generator(ctx, t)
	nodeB := generator(ctx, t)

	_, err := nodeA.Bootstrap(bootstrapAddresses(nodeB))
	assert.Nil(t, err)

	nodeB.SetStreamHandler("test/protocol", func(s net.Stream) {
		defer s.Close()
		data, err := ioutil.ReadAll(s)
		require.Nil(t, err)
		_, err = s.Write(data)
		require.Nil(t, err)
	})

	sendMsg := []byte("hi")

	resp, err := nodeA.SendAndReceive(nodeB.PublicKey(), "test/protocol", sendMsg)
	require.Nil(t, err)

	assert.Equal(t, sendMsg, resp)
}
