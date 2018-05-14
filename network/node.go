package network

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/discover"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv5"
	"os"
)

const TopicLength = 4

type TopicType [TopicLength]byte

var NotaryGroupTopic = []byte("qctt")
var NotaryGroupKey = []byte("c8@rtq4XOuqkZwitX1TfWvIkwg88z9rw")

// from here: https://github.com/status-im/status-go/blob/develop/static/config/staticpeers.json
var bootNodes = []string{
	"enode://7ab298cedc4185a894d21d8a4615262ec6bdce66c9b6783878258e0d5b31013d30c9038932432f70e5b2b6a5cd323bf820554fcb22fbc7b45367889522e9c449@51.15.63.93:30303",
	"enode://f59e8701f18c79c5cbc7618dc7bb928d44dc2f5405c7d693dad97da2d8585975942ec6fd36d3fe608bfdc7270a34a4dd00f38cfe96b2baa24f7cd0ac28d382a1@51.15.79.88:30303",
	"enode://e2a3587b7b41acfc49eddea9229281905d252efba0baf565cf6276df17faf04801b7879eead757da8b5be13b05f25e775ab6d857ff264bc53a89c027a657dd10@51.15.45.114:30303",
	"enode://fe991752c4ceab8b90608fbf16d89a5f7d6d1825647d4981569ebcece1b243b2000420a5db721e214231c7a6da3543fa821185c706cbd9b9be651494ec97f56a@51.15.67.119:30303",
	"enode://482484b9198530ee2e00db89791823244ca41dcd372242e2e1297dd06f6d8dd357603960c5ad9cc8dc15fcdf0e4edd06b7ad7db590e67a0b54f798c26581ebd7@51.15.75.138:30303",
	"enode://9e99e183b5c71d51deb16e6b42ac9c26c75cfc95fff9dfae828b871b348354cbecf196dff4dd43567b26c8241b2b979cb4ea9f8dae2d9aacf86649dafe19a39a@51.15.79.176:30303",
	"enode://12d52c3796700fb5acff2c7d96df7bbb6d7109b67f3442ee3d99ac1c197016cddb4c3568bbeba05d39145c59c990cd64f76bc9b00d4b13f10095c49507dd4cf9@51.15.63.110:30303",
	"enode://0f7c65277f916ff4379fe520b875082a56e587eb3ce1c1567d9ff94206bdb05ba167c52272f20f634cd1ebdec5d9dfeb393018bfde1595d8e64a717c8b46692f@51.15.54.150:30303",
	"enode://e006f0b2dc98e757468b67173295519e9b6d5ff4842772acb18fd055c620727ab23766c95b8ee1008dea9e8ef61e83b1515ddb3fb56dbfb9dbf1f463552a7c9f@212.47.237.127:30303",
	"enode://d40871fc3e11b2649700978e06acd68a24af54e603d4333faecb70926ca7df93baa0b7bf4e927fcad9a7c1c07f9b325b22f6d1730e728314d0e4e6523e5cebc2@51.15.132.235:30303",
	"enode://ea37c9724762be7f668e15d3dc955562529ab4f01bd7951f0b3c1960b75ecba45e8c3bb3c8ebe6a7504d9a40dd99a562b13629cc8e5e12153451765f9a12a61d@163.172.189.205:30303",
	"enode://88c2b24429a6f7683fbfd06874ae3f1e7c8b4a5ffb846e77c705ba02e2543789d66fc032b6606a8d8888eb6239a2abe5897ce83f78dcdcfcb027d6ea69aa6fe9@163.172.157.61:30303",
	"enode://ce6854c2c77a8800fcc12600206c344b8053bb90ee3ba280e6c4f18f3141cdc5ee80bcc3bdb24cbc0e96dffd4b38d7b57546ed528c00af6cd604ab65c4d528f6@163.172.153.124:30303",
	"enode://00ae60771d9815daba35766d463a82a7b360b3a80e35ab2e0daa25bdc6ca6213ff4c8348025e7e1a908a8f58411a364fe02a0fb3c2aa32008304f063d8aaf1a2@163.172.132.85:30303",
	"enode://86ebc843aa51669e08e27400e435f957918e39dc540b021a2f3291ab776c88bbda3d97631639219b6e77e375ab7944222c47713bdeb3251b25779ce743a39d70@212.47.254.155:30303",
	"enode://a1ef9ba5550d5fac27f7cbd4e8d20a643ad75596f307c91cd6e7f85b548b8a6bf215cca436d6ee436d6135f9fe51398f8dd4c0bd6c6a0c332ccb41880f33ec12@51.15.218.125:30303",
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

		SymKeyHash:      whispMessage.SymKeyHash,   // The Keccak256Hash of the key, associated with the Topic
		EnvelopeHash:    whispMessage.EnvelopeHash, // Message envelope hash to act as a unique id
		EnvelopeVersion: whispMessage.EnvelopeVersion,
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

func (c *Node) Start() error {
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
	whisp.AddKeyPair(c.key)
	whisp.Start(nil)

	srv := &p2p.Server{
		Config: p2p.Config{
			MaxPeers:   20,
			PrivateKey: c.key,
			//ListenAddr: ":8000",
			Protocols:      whisp.Protocols(),
			BootstrapNodes: peers,
		},
	}
	if err := srv.Start(); err != nil {
		fmt.Println("could not start server:", err)
		return fmt.Errorf("error starting network node: %v", err)
	}

	c.srv = srv
	c.whisper = whisp

	return nil
}

func (c *Node) Stop() {
	c.whisper.Stop()
	c.srv.Stop()
}

func (c *Node) Send(params MessageParams) error {
	msg, err := whisper.NewSentMessage(params.toWhisper())
	if err != nil {
		return fmt.Errorf("error generating message: %v", err)
	}
	env, err := msg.Wrap(params.toWhisper())
	if err != nil {
		return fmt.Errorf("error wrapping message: %v", err)
	}
	err = c.whisper.Send(env)
	if err != nil {
		return fmt.Errorf("error sending env: %v", err)
	}
	return nil
}

func (c *Node) SubscribeToTopic(topic []byte, symKey []byte) *Subscription {
	topicBytes := whisper.BytesToTopic(topic)

	sub := &Subscription{
		whisperSubscription: &whisper.Filter{
			KeySym:   symKey,
			Topics:   [][]byte{topicBytes[:]},
			AllowP2P: false,
		},
	}

	_, err := c.whisper.Subscribe(sub.whisperSubscription)
	if err != nil {
		panic(fmt.Sprintf("error subscribing: %v", err))
	}
	return sub
}

func (c *Node) SubscribeToKey(key *ecdsa.PrivateKey) *Subscription {
	topicBytes := whisper.BytesToTopic(NotaryGroupTopic)

	sub := &Subscription{
		whisperSubscription: &whisper.Filter{
			Topics:   [][]byte{topicBytes[:]},
			AllowP2P: true,
			KeyAsym:  key,
		},
	}

	c.whisper.Subscribe(sub.whisperSubscription)
	return sub

}
