// +build integration

package network

import (
	"context"
	"fmt"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/google/uuid"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNode_Integration(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	boostrapHost := newBootstrapHost(ctx, t)

	listenerKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	clientKey, err := crypto.GenerateKey()
	assert.Nil(t, err)

	listener := NewNode(listenerKey)
	listener.BoostrapNodes = bootstrapAddresses(boostrapHost)
	listener.Start()
	defer listener.Stop()

	client := NewNode(clientKey)
	client.BoostrapNodes = bootstrapAddresses(boostrapHost)
	client.Start()
	defer client.Stop()

	req := Request{
		Type:    "PING",
		Id:      uuid.New().String(),
		Payload: []byte("PONG"),
	}

	sw := &safewrap.SafeWrap{}
	node := sw.WrapObject(req)
	if sw.Err != nil {
		t.Fatalf("error wrapping: %v", sw.Err)
	}

	params := MessageParams{
		Destination: &listenerKey.PublicKey,
		Payload:     node.RawData(),
		Source:      clientKey,
	}

	log.Debug("sending", "params", params)

	err = client.Send(params)

	require.Nil(t, err)

	err = client.Send(MessageParams{
		Destination: &listenerKey.PublicKey,
		Payload:     []byte("hiKey"),
	})

	require.Nil(t, err)
	fmt.Printf("getting messages\n")

	msg0 := <-listener.MessageChan
	msg1 := <-listener.MessageChan

	assert.Equal(t, msg0.Payload, node.RawData())

	assert.Equal(t, msg1.Payload, []byte("hiKey"))
}
