// +build integration

package network

import (
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

var TestTopic = []byte("test")
var TestKey = []byte("c8@ttq4UOuqkZwitX1TfWvIkwg88z9rw")

func TestRequestHandler_Start(t *testing.T) {
	key, err := crypto.GenerateKey()
	assert.Nil(t, err)

	dstKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	node := NewNode(key)
	node.Start()
	defer node.Stop()

	time.Sleep(1 * time.Second)

	reqHandler := func(req Request) (*Response, error) {
		resp := &Response{
			Payload: req.Payload,
		}
		return resp, nil
	}

	server := NewRequestHandler(node)
	server.AssignHandler("PING", reqHandler)

	server.HandleKey(dstKey)
	server.HandleTopic(TestTopic, TestKey)

	server.Start()

	clientKey, _ := crypto.GenerateKey()

	client := NewClient(clientKey, TestTopic, TestKey)
	client.Start()

	time.Sleep(10 * time.Second)

	respChan, err := client.DoRequest(&Request{
		Type:    "PING",
		Id:      uuid.New().String(),
		Payload: "PONG",
	})

	assert.Nil(t, err)

	assert.IsType(t, &Response{}, <-respChan)

}
