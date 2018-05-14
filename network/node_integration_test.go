// +build integration

package network

import (
	"testing"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/ethereum/go-ethereum/log"
	"time"
)

func TestClient_Integration(t *testing.T) {
	key,err := crypto.GenerateKey()
	assert.Nil(t,err)

	dstKey,err := crypto.GenerateKey()
	assert.Nil(t,err)

	client := NewNode(key)
	client.Start()
	defer client.Stop()

	topicSub := client.SubscribeToTopic(NotaryGroupTopic, NotaryGroupKey)
	keySub := client.SubscribeToKey(dstKey)

	time.Sleep(1 * time.Second)

	err = client.Send(MessageParams{
		Topic: NotaryGroupTopic,
		KeySym: NotaryGroupKey,
		PoW: 0.1,
		WorkTime: 10,
		Payload: []byte("hi"),
	})

	assert.Nil(t, err)

	err = client.Send(MessageParams{
		Topic: NotaryGroupTopic,
		Dst: &dstKey.PublicKey,
		PoW: 0.1,
		WorkTime: 10,
		Payload: []byte("hiKey"),
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