package network

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"os"
	"strings"

	"github.com/quorumcontrol/qc3/p2p"
)

var defaultBootstrapNodes = []string{
	"/ip4/192.168.2.78/tcp/36375/ipfs/QmfLJENuhpynUec1MkgmGqkRUFXHUpEH8bGJLmuYJLfeeV",
}

type MessageParams struct {
	Source      *ecdsa.PrivateKey
	Destination *ecdsa.PublicKey
	Payload     []byte
}

type ReceivedMessage struct {
	Raw []byte

	Payload   []byte
	Padding   []byte
	Signature []byte

	Source      *ecdsa.PublicKey // Message recipient (identity used to decode the message)
	Destination *ecdsa.PublicKey // Message recipient (identity used to decode the message)
}

type Node struct {
	host    p2p.Host
	key     *ecdsa.PrivateKey
	started bool
}

// func (s *Subscription) RetrieveMessages() []*ReceivedMessage {
// 	whisperMsgs := s.whisperSubscription.Retrieve()
// 	msgs := make([]*ReceivedMessage, len(whisperMsgs))
// 	for i, msg := range whisperMsgs {
// 		msgs[i] = fromWhisper(msg)
// 	}
// 	return msgs
// }

func NewNode(key *ecdsa.PrivateKey) *Node {
	ctx := context.Background()
	host, err := p2p.NewHost(ctx, key, 0)

	if err != nil {
		panic(fmt.Sprintf("Could not create node %v", err))
	}

	return &Node{
		host: host,
		key:  key,
	}
}

func bootstrapNodes() []string {
	if envSpecifiedNodes, ok := os.LookupEnv("TUPELO_BOOTSTRAP_NODES"); ok {
		return strings.Split(envSpecifiedNodes, ",")
	}

	return defaultBootstrapNodes
}

func (n *Node) Start() error {
	if n.started == true {
		return nil
	}

	_, err := n.host.Bootstrap(bootstrapNodes())
	if err != nil {
		return err
	}

	n.started = true
	return nil
}

func (n *Node) Stop() {
	if !n.started {
		return
	}
	// Bootstrap proc close?
}

func (n *Node) Send(params MessageParams) error {
	return n.host.Send(params.Destination, params.Payload)
}

func (n *Node) SetHandler(handler func([]byte)) {
	n.host.SetHandler(handler)
}

// func (n *Node) SubscribeToTopic(topic []byte, symKey []byte) *Subscription {
// topicBytes := whisper.BytesToTopic(topic)

// sub := &Subscription{
// 	whisperSubscription: &whisper.Filter{
// 		KeySym:   symKey,
// 		Topics:   [][]byte{topicBytes[:]},
// 		AllowP2P: false,
// 	},
// }

// _, err := n.whisper.Subscribe(sub.whisperSubscription)
// if err != nil {
// 	panic(fmt.Sprintf("error subscribing: %v", err))
// }
// return sub
// }

// func (n *Node) SubscribeToKey(key *ecdsa.PrivateKey) *Subscription {
// sub := &Subscription{
// 	whisperSubscription: &whisper.Filter{
// 		AllowP2P: true,
// 		KeyAsym:  key,
// 	},
// }

// _, err := n.whisper.Subscribe(sub.whisperSubscription)
// if err != nil {
// 	panic(fmt.Sprintf("error subscribing: %v", err))
// }
// return sub
// }
