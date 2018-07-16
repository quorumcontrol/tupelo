// +build integration

package network

import (
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/google/uuid"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/stretchr/testify/assert"
)

func TestNode_Integration(t *testing.T) {
	listenerKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	clientKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	listener := NewNode(listenerKey)
	listener.Start()
	defer listener.Stop()

	client := NewNode(clientKey)
	client.Start()
	defer client.Stop()

	topicSub := listener.SubscribeToTopic(TestTopic, TestKey)
	keySub := listener.SubscribeToKey(clientKey)

	req := Request{
		Type:    "PING",
		Id:      uuid.New().String(),
		Payload: []byte("PONG"),
	}

	sw := &dag.SafeWrap{}
	node := sw.WrapObject(req)
	if sw.Err != nil {
		t.Fatalf("error wrapping: %v", sw.Err)
	}

	params := MessageParams{
		Topic:    TestTopic,
		KeySym:   TestKey,
		PoW:      0.1,
		WorkTime: 2,
		Payload:  node.RawData(),
		TTL:      5,
		Src:      clientKey,
	}

	time.Sleep(5 * time.Second)

	log.Debug("sending", "params", params)

	err = client.Send(params)

	assert.Nil(t, err)

	err = client.Send(MessageParams{
		Topic:    TestTopic,
		Dst:      &clientKey.PublicKey,
		PoW:      0.1,
		WorkTime: 10,
		Payload:  []byte("hiKey"),
		TTL:      5,
	})

	assert.Nil(t, err)

	time.Sleep(5 * time.Second)

	msgs := topicSub.RetrieveMessages()
	log.Debug("msgs:", "msgs", msgs)
	assert.Len(t, msgs, 1)
	//assert.Equal(t, msgs[0].Payload, node.RawData())

	msgs = keySub.RetrieveMessages()
	log.Debug("msgs:", "msgs", msgs)
	assert.Len(t, msgs, 1)

	//assert.Equal(t, msgs[0].Payload, []byte("hiKey"))
}
