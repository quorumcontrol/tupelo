package network

import (
	"crypto/ecdsa"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv6"
)

const TopicLength = 4

type TopicType [TopicLength]byte

var NotaryGroupTopic = []byte("qctt")
var NotaryGroupKey = []byte("c8@rtq4XOuqkZwitX1TfWvIkwg88z9rw")

// From here: https://github.com/status-im/status-go/blob/38a60135b2d7d31a3a00d4e0b519e2dadbf728ff/params/cluster.go
var bootNodes = []string{
	"enode://761a6274bb6559a4ec86ffd49b5162b9d0d431ad377c1dd6447c0051567c02bcd119ec1e55ffd4582ad863e46a36d51ba8e4ec1023898c02baa4f9d8e86b625e@51.15.78.52:30379",
	"enode://f7be0e3c7fae87eeacad42d41d1e1fcfca6a0f7064bae2fea7d65eaf048fb9003649a43ee8e4cb6161bd800a58f628f18a44dbdd38ff37bf4ee8b2be72ea8093@51.15.45.207:30379",
	"enode://7bc0fddc1d45220c0f5e6dea88009bfe93ef99e6dd6a9c8c615b1ce479dc9877cd5250b2a0ffb57bc1d67bea9c4c79829915ae4990706f3da56e869a9d993232@51.15.114.122:30379",
}

type MessageParams struct {
	TTL         uint32
	Source      *ecdsa.PrivateKey
	Destination *ecdsa.PublicKey
	KeySym      []byte
	Topic       []byte
	WorkTime    uint32
	PoW         float64
	Payload     []byte
	//Padding  []byte
}

type ReceivedMessage struct {
	Raw []byte

	Payload   []byte
	Padding   []byte
	Signature []byte

	PoW         float64          // Proof of work as described in the Whisper spec
	Sent        uint32           // Time when the message was posted into the network
	TTL         uint32           // Maximum time to live allowed for the message
	Source      *ecdsa.PublicKey // Message recipient (identity used to decode the message)
	Destination *ecdsa.PublicKey // Message recipient (identity used to decode the message)
	Topic       TopicType

	SymKeyHash      common.Hash // The Keccak256Hash of the key, associated with the Topic
	EnvelopeHash    common.Hash // Message envelope hash to act as a unique id
	EnvelopeVersion uint64
}

func (mp *MessageParams) toWhisper() *whisper.MessageParams {
	return &whisper.MessageParams{
		TTL:      mp.TTL,
		Src:      mp.Source,
		Dst:      mp.Destination,
		KeySym:   mp.KeySym,
		Topic:    whisper.BytesToTopic(mp.Topic),
		WorkTime: mp.WorkTime,
		PoW:      mp.PoW,
		Payload:  mp.Payload,
	}
}

func fromWhisper(whispMessage *whisper.ReceivedMessage) *ReceivedMessage {
	return &ReceivedMessage{
		Raw:       whispMessage.Raw,
		Payload:   whispMessage.Payload,
		Padding:   whispMessage.Padding,
		Signature: whispMessage.Signature,

		PoW:         whispMessage.PoW,  // Proof of work as described in the Whisper spec
		Sent:        whispMessage.Sent, // Time when the message was posted into the network
		TTL:         whispMessage.TTL,  // Maximum time to live allowed for the message
		Source:      whispMessage.Src,  // Message recipient (identity used to decode the message)
		Destination: whispMessage.Dst,  // Message recipient (identity used to decode the message)
		Topic:       TopicType(whispMessage.Topic),

		SymKeyHash:   whispMessage.SymKeyHash,   // The Keccak256Hash of the key, associated with the Topic
		EnvelopeHash: whispMessage.EnvelopeHash, // Message envelope hash to act as a unique id
	}
}

type Node struct {
	whisper *whisper.Whisper
	key     *ecdsa.PrivateKey
	srv     *p2p.Server
	started bool
}

type Subscription struct {
	whisperSubscription *whisper.Filter
}

func (s *Subscription) RetrieveMessages() []*ReceivedMessage {
	whisperMsgs := s.whisperSubscription.Retrieve()
	msgs := make([]*ReceivedMessage, len(whisperMsgs))
	for i, msg := range whisperMsgs {
		msgs[i] = fromWhisper(msg)
	}
	return msgs
}

func NewNode(key *ecdsa.PrivateKey) *Node {
	whisp := whisper.New(&whisper.Config{
		MaxMessageSize:     whisper.MaxMessageSize,
		MinimumAcceptedPOW: 0.001,
	})
	whisp.AddKeyPair(key)

	return &Node{
		key:     key,
		whisper: whisp,
	}
}

func (n *Node) Start() error {
	if n.started == true {
		return nil
	}
	n.started = true

	var peers []*discover.Node
	for _, enode := range bootNodes {
		peer := discover.MustParseNode(enode)
		peers = append(peers, peer)
	}

	n.whisper.Start(nil)

	srv := &p2p.Server{
		Config: p2p.Config{
			MaxPeers:   20,
			PrivateKey: n.key,
			//ListenAddr: ":8000",
			Protocols:      n.whisper.Protocols(),
			BootstrapNodes: peers,
		},
	}
	if err := srv.Start(); err != nil {
		fmt.Println("could not start server:", err)
		return fmt.Errorf("error starting network node: %v", err)
	}

	n.srv = srv

	return nil
}

func (n *Node) Stop() {
	if !n.started {
		return
	}
	n.started = false
	n.whisper.Stop()
	n.srv.Stop()
}

func (n *Node) Send(params MessageParams) error {
	msg, err := whisper.NewSentMessage(params.toWhisper())
	if err != nil {
		return fmt.Errorf("error generating message: %v", err)
	}
	env, err := msg.Wrap(params.toWhisper())
	if err != nil {
		return fmt.Errorf("error wrapping message: %v", err)
	}
	err = n.whisper.Send(env)
	if err != nil {
		return fmt.Errorf("error sending env: %v", err)
	}
	return nil
}

func (n *Node) SubscribeToTopic(topic []byte, symKey []byte) *Subscription {
	topicBytes := whisper.BytesToTopic(topic)

	sub := &Subscription{
		whisperSubscription: &whisper.Filter{
			KeySym:   symKey,
			Topics:   [][]byte{topicBytes[:]},
			AllowP2P: false,
		},
	}

	_, err := n.whisper.Subscribe(sub.whisperSubscription)
	if err != nil {
		panic(fmt.Sprintf("error subscribing: %v", err))
	}
	return sub
}

func (n *Node) SubscribeToKey(key *ecdsa.PrivateKey) *Subscription {
	sub := &Subscription{
		whisperSubscription: &whisper.Filter{
			AllowP2P: true,
			KeyAsym:  key,
		},
	}

	_, err := n.whisper.Subscribe(sub.whisperSubscription)
	if err != nil {
		panic(fmt.Sprintf("error subscribing: %v", err))
	}
	return sub

}
