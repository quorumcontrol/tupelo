package gossip2client

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	logging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	protocol "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-protocol"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/gossip2"
	"github.com/quorumcontrol/qc3/p2p"
	"github.com/tinylib/msgp/msgp"
)

type GossipClient struct {
	Host   *p2p.Host
	cancel context.CancelFunc
}

var log = logging.Logger("gossip2client")

func NewGossipClient(group *consensus.NotaryGroup, boostrapNodes []string) *GossipClient {
	sessionKey, err := crypto.GenerateKey()
	if err != nil {
		panic("error generating key")
	}

	ctx, cancel := context.WithCancel(context.Background())
	host, err := p2p.NewHost(ctx, sessionKey, p2p.GetRandomUnusedPort())
	host.Bootstrap(boostrapNodes)

	return &GossipClient{
		Host:   host,
		cancel: cancel,
	}
}

func (gc *GossipClient) Stop() {
	gc.cancel()
}

func (gc *GossipClient) Send(publicKey *ecdsa.PublicKey, protocol protocol.ID, payload []byte, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	stream, err := gc.Host.NewStream(ctx, publicKey, protocol)
	if err != nil {
		return fmt.Errorf("Error opening new stream: %v", err)
	}

	n, err := stream.Write(payload)
	if err != nil {
		return fmt.Errorf("Error writing message: %v", err)
	}
	log.Debugf("%s wrote %d bytes", gc.Host.P2PIdentity(), n)
	stream.Close()

	return nil
}

type SubscribeResponse struct {
	State *gossip2.CurrentState
	Error error
}

func (gc *GossipClient) Subscribe(publicKey *ecdsa.PublicKey, treeDid string, timeout time.Duration) (chan *SubscribeResponse, error) {
	ch := make(chan *SubscribeResponse, 1)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)

	stream, err := gc.Host.NewStream(ctx, publicKey, gossip2.ChainTreeChangeProtocol)
	if err != nil {
		cancel()
		return ch, err
	}

	reader := msgp.NewReader(stream)
	writer := msgp.NewWriter(stream)

	subscriptionReq := &gossip2.ChainTreeSubscriptionRequest{ObjectID: []byte(treeDid)}
	err = subscriptionReq.EncodeMsg(writer)
	if err != nil {
		cancel()
		return ch, err
	}
	err = writer.Flush()
	if err != nil {
		cancel()
		return ch, err
	}

	var initialResp gossip2.ProtocolMessage
	err = initialResp.DecodeMsg(reader)

	if err != nil {
		cancel()
		return ch, err
	}

	if initialResp.Code == 503 {
		cancel()
		return ch, err
	}

	go func(ch chan *SubscribeResponse, reader *msgp.Reader, stream net.Stream, cancel context.CancelFunc) {
		defer stream.Close()
		defer cancel()

		var resp SubscribeResponse
		var msg gossip2.ProtocolMessage
		err = msg.DecodeMsg(reader)
		if err != nil {
			resp.Error = err
		}
		currentState, err := gossip2.FromProtocolMessage(&msg)
		if err != nil {
			resp.Error = err
		}

		if err == nil {
			resp.State = currentState.(*gossip2.CurrentState)
		}

		ch <- &resp
	}(ch, reader, stream, cancel)

	return ch, err
}
