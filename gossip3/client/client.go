package client

import (
	"bytes"
	"crypto/ecdsa"
	"fmt"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/Workiva/go-datastructures/bitarray"
	"github.com/ethereum/go-ethereum/crypto"
	cid "github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/types"
	"go.uber.org/zap"
)

type Client struct {
	Group            *types.NotaryGroup
	log              *zap.SugaredLogger
	subscriberActors []*actor.PID
}

type subscriberActor struct {
	middleware.LogAwareHolder

	ch      chan interface{}
	timeout time.Duration
}

func newSubscriberActorProps(ch chan interface{}, timeout time.Duration) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &subscriberActor{
			ch:      ch,
			timeout: timeout,
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (sa *subscriberActor) Receive(ctx actor.Context) {
	switch msg := ctx.Message().(type) {
	case *actor.Started:
		ctx.SetReceiveTimeout(sa.timeout)
	case *actor.ReceiveTimeout:
		ctx.Self().Poison()
		close(sa.ch)
	case *actor.Terminated:
		// For some reason we never seem to receive this when we timeout and self-poison
		close(sa.ch)
	case *messages.CurrentState:
		sa.ch <- msg
		ctx.Respond(&messages.TipSubscription{
			ObjectID:    msg.Signature.ObjectID,
			TipValue:    msg.Signature.NewTip,
			Unsubscribe: true,
		})
		ctx.Self().Poison()
	case *messages.Error:
		sa.ch <- msg
		ctx.Self().Poison()
	}
}

func New(group *types.NotaryGroup) *Client {
	return &Client{
		Group: group,
		log:   middleware.Log.Named("client"),
	}
}

func (c *Client) Stop() {
	for _, act := range c.subscriberActors {
		act.Stop()
	}
}

func (c *Client) TipRequest(chainID string) (*messages.CurrentState, error) {
	target := c.Group.GetRandomSyncer()
	fut := actor.NewFuture(10 * time.Second)
	target.Request(&messages.GetTip{
		ObjectID: []byte(chainID),
	}, fut.PID())
	res, err := fut.Result()
	if err != nil {
		return nil, fmt.Errorf("error getting tip: %v", err)
	}
	return res.(*messages.CurrentState), nil
}

func (c *Client) Subscribe(signer *types.Signer, treeDid string, expectedTip cid.Cid, timeout time.Duration) (chan interface{}, error) {
	ch := make(chan interface{}, 1)
	act, err := actor.SpawnPrefix(newSubscriberActorProps(ch, timeout), "sub-"+treeDid)
	if err != nil {
		return nil, fmt.Errorf("error spawning: %v", err)
	}
	c.subscriberActors = append(c.subscriberActors, act)
	signer.Actor.Request(&messages.TipSubscription{
		ObjectID: []byte(treeDid),
		TipValue: expectedTip.Bytes(),
	}, act)
	return ch, nil
}

func (c *Client) SendTransaction(signer *types.Signer, trans *messages.Transaction) error {
	value, err := trans.MarshalMsg(nil)
	if err != nil {
		return fmt.Errorf("error marshaling: %v", err)
	}
	key := crypto.Keccak256(value)
	signer.Actor.Tell(&messages.Store{
		Key:   key,
		Value: value,
	})
	return nil
}

func (c *Client) PlayTransactions(tree *consensus.SignedChainTree, treeKey *ecdsa.PrivateKey, remoteTip *cid.Cid, transactions []*chaintree.Transaction) (*consensus.AddBlockResponse, error) {
	sw := safewrap.SafeWrap{}

	if remoteTip != nil && cid.Undef.Equals(*remoteTip) {
		remoteTip = nil
	}

	root, err := getRoot(tree)
	if err != nil {
		return nil, fmt.Errorf("error getting root: %v", err)
	}

	var height uint64

	if tree.IsGenesis() {
		height = 0
	} else {
		height = root.Height + 1
	}

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			Height:       height,
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

	newTree, err := chaintree.NewChainTree(tree.ChainTree.Dag, tree.ChainTree.BlockValidators, tree.ChainTree.Transactors)
	if err != nil {
		return nil, fmt.Errorf("error creating new tree: %v", err)
	}
	valid, err := newTree.ProcessBlock(blockWithHeaders)
	if !valid || err != nil {
		return nil, fmt.Errorf("error processing block (valid: %t): %v", valid, err)
	}

	expectedTip := newTree.Dag.Tip

	transaction := messages.Transaction{
		PreviousTip: storedTip.Bytes(),
		Height:      blockWithHeaders.Height,
		Payload:     sw.WrapObject(blockWithHeaders).RawData(),
		NewTip:      expectedTip.Bytes(),
		ObjectID:    []byte(tree.MustId()),
		State:       nodes,
	}

	var target *types.Signer
	previousSig, ok := tree.Signatures[c.Group.ID]
	if ok {
		target = c.Group.GetRandomSignerWithIndexMask(previousSig.Signers)
	} else {
		target = c.Group.GetRandomSigner()
	}

	respChan, err := c.Subscribe(target, tree.MustId(), expectedTip, 60*time.Second)
	if err != nil {
		return nil, fmt.Errorf("error subscribing: %v", err)
	}

	err = c.SendTransaction(target, &transaction)
	if err != nil {
		panic(fmt.Errorf("error sending transaction %v", err))
	}

	uncastResp := <-respChan

	if uncastResp == nil {
		return nil, fmt.Errorf("error timeout")
	}

	var resp *messages.CurrentState
	switch respVal := uncastResp.(type) {
	case *messages.Error:
		return nil, fmt.Errorf("error response: %v", respVal)
	case *messages.CurrentState:
		resp = respVal
	default:
		return nil, fmt.Errorf("error unrecognized response type: %T", respVal)
	}

	if !bytes.Equal(resp.Signature.NewTip, expectedTip.Bytes()) {
		respCid, _ := cid.Cast(resp.Signature.NewTip)
		return nil, fmt.Errorf("error, tree updated to different tip - expected: %v - received: %v", expectedTip.String(), respCid.String())
	}

	success, err := tree.ChainTree.ProcessBlock(blockWithHeaders)
	if !success || err != nil {
		return nil, fmt.Errorf("error processing block (valid: %t): %v", valid, err)
	}

	consensusSig, err := toConsensusSig(resp.Signature, c.Group)
	if err != nil {
		return nil, fmt.Errorf("error converting sig: %v", err)
	}

	tree.Signatures[c.Group.ID] = *consensusSig

	newCid, err := cid.Cast(resp.Signature.NewTip)
	if err != nil {
		return nil, fmt.Errorf("error new tip is not parsable CID %v", string(resp.Signature.NewTip))
	}

	addResponse := &consensus.AddBlockResponse{
		ChainId:   tree.MustId(),
		Tip:       &newCid,
		Signature: tree.Signatures[c.Group.ID],
	}

	if tree.Signatures == nil {
		tree.Signatures = make(consensus.SignatureMap)
	}

	return addResponse, nil
}

func toConsensusSig(sig *messages.Signature, ng *types.NotaryGroup) (*consensus.Signature, error) {
	signersBitArray, err := bitarray.Unmarshal(sig.Signers)
	if err != nil {
		return nil, fmt.Errorf("error getting bit array: %v", err)
	}

	signersSlice := make([]bool, len(ng.AllSigners()), len(ng.AllSigners()))
	for i := range ng.AllSigners() {
		isSet, err := signersBitArray.GetBit(uint64(i))
		if err != nil {
			return nil, fmt.Errorf("error getting bit: %v", err)
		}
		signersSlice[i] = isSet
	}

	return &consensus.Signature{
		Signers:   signersSlice,
		Signature: sig.Signature,
		Type:      consensus.KeyTypeBLSGroupSig,
	}, nil
}

func getRoot(sct *consensus.SignedChainTree) (*chaintree.RootNode, error) {
	ct := sct.ChainTree
	unmarshaledRoot, err := ct.Dag.Get(ct.Dag.Tip)
	if unmarshaledRoot == nil || err != nil {
		return nil, fmt.Errorf("error,missing root: %v", err)
	}

	root := &chaintree.RootNode{}

	err = cbornode.DecodeInto(unmarshaledRoot.RawData(), root)
	if err != nil {
		return nil, fmt.Errorf("error decoding root: %v", err)
	}
	return root, nil
}
