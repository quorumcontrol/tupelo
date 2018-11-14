package gossip2client

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	logging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
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

func (gc *GossipClient) Subscribe(publicKey *ecdsa.PublicKey, treeDid string, timeout time.Duration) (*gossip2.CurrentState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	stream, err := gc.Host.NewStream(ctx, publicKey, gossip2.ChainTreeChangeProtocol)
	if err != nil {
		return nil, err
	}
	defer stream.Close()

	reader := msgp.NewReader(stream)
	writer := msgp.NewWriter(stream)

	subscriptionReq := &gossip2.ChainTreeSubscriptionRequest{ObjectID: []byte(treeDid)}
	err = subscriptionReq.EncodeMsg(writer)
	if err != nil {
		return nil, err
	}
	err = writer.Flush()
	if err != nil {
		return nil, err
	}

	var resp gossip2.ProtocolMessage
	err = resp.DecodeMsg(reader)

	if err != nil {
		return nil, err
	}

	if resp.Code == 503 {
		return nil, fmt.Errorf("Could not bind subscription for %s, host is busy", treeDid)
	}

	var newResp gossip2.ProtocolMessage
	err = newResp.DecodeMsg(reader)
	if err != nil {
		return nil, err
	}
	currentState, err := gossip2.FromProtocolMessage(&newResp)
	if err != nil {
		return nil, err
	}

	return currentState.(*gossip2.CurrentState), nil
}
