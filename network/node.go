package network

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/btcsuite/btcd/btcec"
	crypto "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-crypto"
	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/quorumcontrol/tupelo/p2p"
)

const ProtocolID = "tupelo/0.1"

var defaultBootstrapNodes = []string{
	"/ip4/18.196.112.81/tcp/34001/ipfs/16Uiu2HAmJXuoQMRqg4bShcBTczUMn8zMyCvXAPuefCtqZb21iih8",
	"/ip4/34.231.17.217/tcp/34001/ipfs/16Uiu2HAmLos2gmQLkVkiYF3JJBDW2WqZxCHoMb2fLmo77a2tqExF",
}

type MessageParams struct {
	Source      *ecdsa.PrivateKey
	Destination *ecdsa.PublicKey
	Payload     []byte
}

type ReceivedMessage struct {
	Payload []byte

	Source *ecdsa.PublicKey // Message recipient (identity used to decode the message)
}

type Node struct {
	BoostrapNodes []string
	MessageChan   chan ReceivedMessage
	host          *p2p.Host
	key           *ecdsa.PrivateKey
	started       bool
	cancel        context.CancelFunc
}

func NewNode(key *ecdsa.PrivateKey) *Node {
	ctx, cancel := context.WithCancel(context.Background())
	host, err := p2p.NewHost(ctx, key, 0)

	if err != nil {
		panic(fmt.Sprintf("Could not create node %v", err))
	}

	node := &Node{
		host:          host,
		key:           key,
		cancel:        cancel,
		BoostrapNodes: BootstrapNodes(),
		MessageChan:   make(chan ReceivedMessage, 10),
	}

	host.SetStreamHandler(ProtocolID, node.handler)

	return node
}

func BootstrapNodes() []string {
	if envSpecifiedNodes, ok := os.LookupEnv("TUPELO_BOOTSTRAP_NODES"); ok {
		return strings.Split(envSpecifiedNodes, ",")
	}

	return defaultBootstrapNodes
}

func (n *Node) Start() error {
	if n.started == true {
		return nil
	}

	_, err := n.host.Bootstrap(n.BoostrapNodes)
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
	n.cancel()
}

func (n *Node) Send(params MessageParams) error {
	fmt.Printf("sending %s\n", params.Payload)
	return n.host.Send(params.Destination, ProtocolID, params.Payload)
}

func (n *Node) handler(s net.Stream) {
	fmt.Printf("new stream from %v\n", s.Conn().RemotePeer().Pretty())
	data, err := ioutil.ReadAll(s)
	if err != nil {
		fmt.Printf("error reading: %v", err)
	}
	s.Close()
	fmt.Printf("received: %s\n", data)
	n.MessageChan <- ReceivedMessage{
		Payload: data,
		Source:  (*btcec.PublicKey)(s.Conn().RemotePublicKey().(*crypto.Secp256k1PublicKey)).ToECDSA(),
	}
}

func (n *Node) PublicKey() *ecdsa.PublicKey {
	return &n.key.PublicKey
}
