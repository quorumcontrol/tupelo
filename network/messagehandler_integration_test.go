// +build integration

package network

import (
	"context"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

var TestTopic = []byte("test")
var TestKey = []byte("c8@ttq4UOuqkZwitX1TfWvIkwg88z9rw")

func TestClient_DoRequest(t *testing.T) {
	serverKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	clientKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	serverNode := NewNode(serverKey)
	serverNode.Start()
	defer serverNode.Stop()

	clientNode := NewNode(clientKey)
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
	server.Start()
	defer server.Stop()

	client := NewMessageHandler(clientNode, TestTopic)
	client.Start()
	defer client.Stop()

	time.Sleep(1 * time.Second)

	respChan, err := client.DoRequest(&serverKey.PublicKey, &Request{
		Type:    "PING",
		Id:      uuid.New().String(),
		Payload: []byte("PONG"),
	})

	assert.Nil(t, err)

	resp := <-respChan

	assert.IsType(t, &Response{}, resp)
	assert.Equal(t, []byte("PONG"), resp.Payload)
}

func TestClient_Broadcast(t *testing.T) {
	serverKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	clientKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	serverNode := NewNode(serverKey)
	serverNode.Start()
	defer serverNode.Stop()

	clientNode := NewNode(clientKey)
	clientNode.Start()
	defer serverNode.Stop()

	reqHandler := func(_ context.Context, req Request, respChan ResponseChan) error {
		assert.Equal(t, req.Payload, []byte("PONG"))
		return nil
	}

	server := NewMessageHandler(serverNode, TestTopic)
	server.AssignHandler("PING", reqHandler)
	server.HandleTopic(TestTopic, TestKey)
	server.Start()
	defer server.Stop()

	client := NewMessageHandler(clientNode, TestTopic)
	client.Start()
	defer client.Stop()

	time.Sleep(1 * time.Second)

	err = client.Broadcast(TestTopic, TestKey, &Request{
		Type:    "PING",
		Id:      uuid.New().String(),
		Payload: []byte("PONG"),
	})
	assert.Nil(t, err)
}

func TestClient_Push(t *testing.T) {
	serverKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	clientKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	serverNode := NewNode(serverKey)
	serverNode.Start()
	defer serverNode.Stop()

	clientNode := NewNode(clientKey)
	clientNode.Start()
	defer serverNode.Stop()

	var received Request

	reqHandler := func(_ context.Context, req Request, respChan ResponseChan) error {
		received = req
		respChan <- nil
		return nil
	}

	server := NewMessageHandler(serverNode, TestTopic)
	server.AssignHandler("PING", reqHandler)
	server.HandleTopic(TestTopic, TestKey)
	server.Start()
	defer server.Stop()

	client := NewMessageHandler(clientNode, TestTopic)
	client.Start()
	defer client.Stop()

	time.Sleep(1 * time.Second)

	err = client.Push(&serverKey.PublicKey, &Request{
		Type:    "PING",
		Id:      uuid.New().String(),
		Payload: []byte("PONG"),
	})

	time.Sleep(2 * time.Second)

	assert.Equal(t, received.Payload, []byte("PONG"))

	assert.Nil(t, err)
}
