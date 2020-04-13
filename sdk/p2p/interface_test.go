package p2p

import (
	"context"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type nodeGenerator func(ctx context.Context, t *testing.T) Node

func NodeTests(t *testing.T, generator nodeGenerator) {
	BootstrapTest(t, generator)
	SendTest(t, generator)
	SendAndReceiveTest(t, generator)
	PubSubTest(t, generator)
	PeerDiscovery(t, generator)
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
	require.Nil(t, err)
	err = nodeA.WaitForBootstrap(1, 1*time.Second)
	require.Nil(t, err)

	msgs := make(chan []byte, 1)

	nodeB.SetStreamHandler("test/protocol", func(s network.Stream) {
		defer s.Close()
		data, err := ioutil.ReadAll(s)
		require.Nil(t, err)
		msgs <- data
	})

	err = nodeA.Send(nodeB.PublicKey(), "test/protocol", []byte("hi"))
	require.Nil(t, err)

	received := <-msgs
	assert.Equal(t, []byte("hi"), received)
}

func SendAndReceiveTest(t *testing.T, generator nodeGenerator) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	nodeA := generator(ctx, t)
	nodeB := generator(ctx, t)

	_, err := nodeA.Bootstrap(bootstrapAddresses(nodeB))
	require.Nil(t, err)
	err = nodeA.WaitForBootstrap(1, 1*time.Second)
	require.Nil(t, err)

	nodeB.SetStreamHandler("test/protocol", func(s network.Stream) {
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

func PeerDiscovery(t *testing.T, generator nodeGenerator) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bootstrapper := generator(ctx, t)
	nodeCount := 5
	nodes := make([]Node, nodeCount)
	for i := 0; i < nodeCount; i++ {
		n := generator(ctx, t)
		nodes[i] = n
		_, err := n.Bootstrap(bootstrapAddresses(bootstrapper))
		require.Nil(t, err)
	}

	err := nodes[0].WaitForBootstrap(nodeCount-1, 10*time.Second) // see that it connects to all the other nodes
	require.Nil(t, err)
}

func PubSubTest(t *testing.T, generator nodeGenerator) {
	ctx, timeoutCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer timeoutCancel()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	bootstrapper := generator(ctx, t)

	nodeA := generator(ctx, t)
	nodeB := generator(ctx, t)

	_, err := nodeA.Bootstrap(bootstrapAddresses(bootstrapper))
	require.Nil(t, err)

	_, err = nodeB.Bootstrap(bootstrapAddresses(bootstrapper))
	require.Nil(t, err)

	err = nodeA.WaitForBootstrap(2, 10*time.Second)
	require.Nil(t, err)

	sub, err := nodeB.GetPubSub().Subscribe("testSubscription")
	require.Nil(t, err)

	time.Sleep(100 * time.Millisecond)

	err = nodeA.GetPubSub().Publish("testSubscription", []byte("hi"))
	require.Nil(t, err)

	msg, err := sub.Next(ctx)
	require.Nil(t, err)
	assert.Equal(t, msg.Data, []byte("hi"))
}
