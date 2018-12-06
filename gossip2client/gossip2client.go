package gossip2client

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"fmt"
	"io/ioutil"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-cid"
	logging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	protocol "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-protocol"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/quorumcontrol/tupelo/gossip2"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/tinylib/msgp/msgp"
)

type GossipClient struct {
	Host   p2p.Node
	Group  *consensus.NotaryGroup
	cancel context.CancelFunc
}

var log = logging.Logger("gossip2client")

func NewGossipClient(group *consensus.NotaryGroup, boostrapNodes []string) *GossipClient {
	sessionKey, err := crypto.GenerateKey()
	if err != nil {
		panic("error generating key")
	}

	ctx, cancel := context.WithCancel(context.Background())
	host, err := p2p.NewLibP2PHost(ctx, sessionKey, 0)
	host.Bootstrap(boostrapNodes)

	return &GossipClient{
		Host:   host,
		Group:  group,
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
	log.Debugf("%s wrote %d bytes", gc.Host.Identity(), n)
	stream.Close()

	return nil
}

func (gc *GossipClient) SendAndReceive(publicKey *ecdsa.PublicKey, protocol protocol.ID, payload []byte, timeout time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	stream, err := gc.Host.NewStream(ctx, publicKey, protocol)
	defer stream.Close()
	if err != nil {
		return nil, fmt.Errorf("Error opening new stream: %v", err)
	}

	n, err := stream.Write(payload)
	if err != nil {
		return nil, fmt.Errorf("Error writing message: %v", err)
	}
	log.Debugf("%s wrote %d bytes", gc.Host.Identity(), n)
	return ioutil.ReadAll(stream)
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

func (gc *GossipClient) TipRequest(chainId string) (*gossip2.CurrentState, error) {
	targetKey, err := gc.randomSignerPublicKey()

	if err != nil {
		return nil, fmt.Errorf("error tip request failed: %v", err)
	}

	tq := gossip2.TipQuery{ObjectID: []byte(chainId)}

	payload, err := tq.MarshalMsg(nil)

	if err != nil {
		return nil, fmt.Errorf("error tip request failed: %v", err)
	}

	resp, err := gc.SendAndReceive(targetKey, gossip2.TipProtocol, payload, 30*time.Second)

	if err != nil {
		return nil, fmt.Errorf("error tip request failed: %v", err)
	}

	var currentState gossip2.CurrentState
	_, err = currentState.UnmarshalMsg(resp)
	if err != nil {
		return nil, fmt.Errorf("error tip request failed: %v", err)
	}

	return &currentState, nil
}

func (gc *GossipClient) PlayTransactions(tree *consensus.SignedChainTree, treeKey *ecdsa.PrivateKey, remoteTip string, transactions []*chaintree.Transaction) (*consensus.AddBlockResponse, error) {
	sw := safewrap.SafeWrap{}

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  remoteTip,
			Transactions: transactions,
		},
	}

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	if err != nil {
		return nil, fmt.Errorf("error signing: %v", err)
	}

	//TODO: only send the necessary nodes
	cborNodes, err := tree.ChainTree.Dag.Nodes()
	if err != nil {
		return nil, fmt.Errorf("error getting nodes: %v", err)
	}
	nodes := make([][]byte, len(cborNodes))
	for i, node := range cborNodes {
		nodes[i] = node.RawData()
	}

	storedTip := tree.Tip()
	addBlockRequest := &consensus.AddBlockRequest{
		Nodes:    nodes,
		NewBlock: blockWithHeaders,
		Tip:      &storedTip,
		ChainId:  tree.MustId(),
	}

	newTree, err := chaintree.NewChainTree(tree.ChainTree.Dag, tree.ChainTree.BlockValidators, tree.ChainTree.Transactors)
	if err != nil {
		return nil, fmt.Errorf("error creating new tree: %v", err)
	}
	valid, err := newTree.ProcessBlock(blockWithHeaders)
	if !valid || err != nil {
		return nil, fmt.Errorf("error processing block (valid: %t): %v", valid, err)
	}

	expectedTip := newTree.Dag.Tip

	transaction := gossip2.Transaction{
		PreviousTip: []byte(remoteTip),
		Payload:     sw.WrapObject(addBlockRequest).RawData(),
		NewTip:      expectedTip.Bytes(),
		ObjectID:    []byte(tree.MustId()),
	}

	targetKey, err := gc.randomSignerPublicKey()
	if err != nil {
		return nil, fmt.Errorf("error fetching target: %v", err)
	}

	payload, err := transaction.MarshalMsg(nil)

	if err != nil {
		return nil, fmt.Errorf("error marshaling transaction: %v", err)
	}

	respChan, err := gc.Subscribe(targetKey, tree.MustId(), 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("error subscribing: %v", err)
	}

	err = gc.Send(targetKey, gossip2.NewTransactionProtocol, payload, 30*time.Second)
	if err != nil {
		panic(fmt.Errorf("Error sending transaction %v", err))
	}

	resp := <-respChan

	if resp.Error != nil {
		return nil, fmt.Errorf("error on response %v", resp.Error)
	}

	if !bytes.Equal(resp.State.Tip, expectedTip.Bytes()) {
		respCid, _ := cid.Cast(resp.State.Tip)
		return nil, fmt.Errorf("error, tree updated to different tip - expected: %v - received: %v", respCid.String(), expectedTip.String())
	}

	success, err := tree.ChainTree.ProcessBlock(blockWithHeaders)
	if !success || err != nil {
		return nil, fmt.Errorf("error processing block (valid: %t): %v", valid, err)
	}

	tree.Signatures[gc.Group.ID] = consensus.Signature{
		Signers:   resp.State.Signature.Signers,
		Signature: resp.State.Signature.Signature,
		Type:      consensus.KeyTypeBLSGroupSig,
	}

	newCid, err := cid.Cast(resp.State.Tip)
	if err != nil {
		return nil, fmt.Errorf("error new tip is not parsable CID %v", string(resp.State.Tip))
	}

	addResponse := &consensus.AddBlockResponse{
		ChainId:   tree.MustId(),
		Tip:       &newCid,
		Signature: tree.Signatures[gc.Group.ID],
	}

	if tree.Signatures == nil {
		tree.Signatures = make(consensus.SignatureMap)
	}

	return addResponse, nil
}

func (gc *GossipClient) randomSignerPublicKey() (*ecdsa.PublicKey, error) {
	roundInfo, err := gc.Group.MostRecentRoundInfo(gc.Group.RoundAt(time.Now()))
	if err != nil {
		return nil, fmt.Errorf("error getting peer: %v", err)
	}
	return roundInfo.RandomMember().DstKey.ToEcdsaPub(), nil
}
