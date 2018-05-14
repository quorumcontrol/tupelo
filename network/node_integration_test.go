// +build integration

package network

import (
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestNode_Integration(t *testing.T) {
	key, err := crypto.GenerateKey()
	assert.Nil(t, err)

	dstKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	client := NewNode(key)
	client.Start()
	defer client.Stop()

	topicSub := client.SubscribeToTopic(TestTopic, TestKey)
	keySub := client.SubscribeToKey(dstKey)

	time.Sleep(1 * time.Second)

	err = client.Send(MessageParams{
		Topic:    TestTopic,
		KeySym:   TestKey,
		PoW:      0.1,
		WorkTime: 10,
		Payload:  []byte("hi"),
	})

	assert.Nil(t, err)

	err = client.Send(MessageParams{
		Topic:    TestTopic,
		Dst:      &dstKey.PublicKey,
		PoW:      0.1,
		WorkTime: 10,
		Payload:  []byte("hiKey"),
	})

	assert.Nil(t, err)

	time.Sleep(1 * time.Second)

	msgs := topicSub.RetrieveMessages()
	log.Debug("msgs:", "msgs", msgs)

	assert.Equal(t, msgs[0].Payload, []byte("hi"))

	msgs = keySub.RetrieveMessages()
	log.Debug("msgs:", "msgs", msgs)

	assert.Equal(t, msgs[0].Payload, []byte("hiKey"))

}
