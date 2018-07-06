package network

import (
	"crypto/ecdsa"
	"fmt"
	"os"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
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
	"enode://a6a2a9b3a7cbb0a15da74301537ebba549c990e3325ae78e1272a19a3ace150d03c184b8ac86cc33f1f2f63691e467d49308f02d613277754c4dccd6773b95e8@206.189.108.68:30304",
	"enode://207e53d9bf66be7441e3daba36f53bfbda0b6099dba9a865afc6260a2d253fb8a56a72a48598a4f7ba271792c2e4a8e1a43aaef7f34857f520c8c820f63b44c8@35.224.15.65:30304",
}

type MessageParams struct {
	TTL      uint32
	Src      *ecdsa.PrivateKey
	Dst      *ecdsa.PublicKey
	KeySym   []byte
	Topic    []byte
	WorkTime uint32
	PoW      float64
	Payload  []byte
	//Padding  []byte
}

type ReceivedMessage struct {
	Raw []byte

	Payload   []byte
	Padding   []byte
	Signature []byte

	PoW   float64          // Proof of work as described in the Whisper spec
	Sent  uint32           // Time when the message was posted into the network
	TTL   uint32           // Maximum time to live allowed for the message
	Src   *ecdsa.PublicKey // Message recipient (identity used to decode the message)
	Dst   *ecdsa.PublicKey // Message recipient (identity used to decode the message)
	Topic TopicType

	SymKeyHash      common.Hash // The Keccak256Hash of the key, associated with the Topic
	EnvelopeHash    common.Hash // Message envelope hash to act as a unique id
	EnvelopeVersion uint64
}

func (mp *MessageParams) toWhisper() *whisper.MessageParams {
	return &whisper.MessageParams{
		TTL:      mp.TTL,
		Src:      mp.Src,
		Dst:      mp.Dst,
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

		PoW:   whispMessage.PoW,  // Proof of work as described in the Whisper spec
		Sent:  whispMessage.Sent, // Time when the message was posted into the network
		TTL:   whispMessage.TTL,  // Maximum time to live allowed for the message
		Src:   whispMessage.Src,  // Message recipient (identity used to decode the message)
		Dst:   whispMessage.Dst,  // Message recipient (identity used to decode the message)
		Topic: TopicType(whispMessage.Topic),

		SymKeyHash:   whispMessage.SymKeyHash,   // The Keccak256Hash of the key, associated with the Topic
		EnvelopeHash: whispMessage.EnvelopeHash, // Message envelope hash to act as a unique id
	}
}

type Node struct {
	whisper *whisper.Whisper
	key     *ecdsa.PrivateKey
	srv     *p2p.Server
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
	return &Node{
		key: key,
	}
}

func (n *Node) Start() error {
	var peers []*discover.Node
	for _, enode := range bootNodes {
		peer := discover.MustParseNode(enode)
		peers = append(peers, peer)
	}

	log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(log.LvlDebug), log.StreamHandler(os.Stderr, log.TerminalFormat(false))))

	whisp := whisper.New(&whisper.Config{
		MaxMessageSize:     whisper.MaxMessageSize,
		MinimumAcceptedPOW: 0.001,
	})
	whisp.AddKeyPair(n.key)
	whisp.Start(nil)

	srv := &p2p.Server{
		Config: p2p.Config{
			MaxPeers:   20,
			PrivateKey: n.key,
			//ListenAddr: ":8000",
			Protocols:      whisp.Protocols(),
			BootstrapNodes: peers,
		},
	}
	if err := srv.Start(); err != nil {
		fmt.Println("could not start server:", err)
		return fmt.Errorf("error starting network node: %v", err)
	}

	n.srv = srv
	n.whisper = whisp

	return nil
}

func (n *Node) Stop() {
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
