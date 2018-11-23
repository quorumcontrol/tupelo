// +build integration

package network

import (
	"context"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var TestTopic = []byte("test")
var TestKey = []byte("c8@ttq4UOuqkZwitX1TfWvIkwg88z9rw")

func TestClient_DoRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	boostrapHost := newBootstrapHost(ctx, t)

	serverKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	clientKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	serverNode := NewNode(serverKey)
	serverNode.BoostrapNodes = bootstrapAddresses(boostrapHost)
	serverNode.Start()
	defer serverNode.Stop()

	clientNode := NewNode(clientKey)
	clientNode.BoostrapNodes = bootstrapAddresses(boostrapHost)
	clientNode.Start()
	defer serverNode.Stop()

	reqHandler := func(_ context.Context, req Request, respChan ResponseChan) error {
		respChan <- &Response{
			Payload: req.Payload,
		}
		return nil
	}

	server := NewMessageHandler(serverNode, TestTopic)
	server.AssignHandler("PING", reqHandler)
	fmt.Println("starting")
	server.Start()
	defer server.Stop()

	client := NewMessageHandler(clientNode, TestTopic)
	client.Start()
	defer client.Stop()
	fmt.Println("do request")
	respChan, err := client.DoRequest(&serverKey.PublicKey, &Request{
		Type:    "PING",
		Id:      uuid.New().String(),
		Payload: []byte("PONG"),
	})

	require.Nil(t, err)

	resp := <-respChan

	assert.IsType(t, &Response{}, resp)
	assert.Equal(t, []byte("PONG"), resp.Payload)
}

func TestClient_Push(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	boostrapHost := newBootstrapHost(ctx, t)

	serverKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	clientKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	serverNode := NewNode(serverKey)
	serverNode.BoostrapNodes = bootstrapAddresses(boostrapHost)
	serverNode.Start()
	defer serverNode.Stop()

	clientNode := NewNode(clientKey)
	clientNode.BoostrapNodes = bootstrapAddresses(boostrapHost)
	clientNode.Start()
	defer serverNode.Stop()

	var received Request
	doneCh := make(chan struct{})

	reqHandler := func(_ context.Context, req Request, respChan ResponseChan) error {
		received = req
		respChan <- nil
		doneCh <- struct{}{}
		return nil
	}

	server := NewMessageHandler(serverNode, TestTopic)
	server.AssignHandler("PING", reqHandler)
	server.Start()
	defer server.Stop()

	client := NewMessageHandler(clientNode, TestTopic)
	client.Start()
	defer client.Stop()

	err = client.Push(&serverKey.PublicKey, &Request{
		Type:    "PING",
		Id:      uuid.New().String(),
		Payload: []byte("PONG"),
	})
	require.Nil(t, err)
	<-doneCh

	assert.Equal(t, []byte("PONG"), received.Payload)

}
