package client

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"time"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	cbornode "github.com/ipfs/go-ipld-cbor"
	format "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"
	"github.com/opentracing/opentracing-go"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/gossip"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/messages/v2/build/go/transactions"

	"github.com/quorumcontrol/tupelo/sdk/consensus"
	"github.com/quorumcontrol/tupelo/sdk/gossip/client/pubsubinterfaces"
	"github.com/quorumcontrol/tupelo/sdk/gossip/types"
)

var ErrTimeout = errors.New("error timeout")
var ErrNotFound = hamt.ErrNotFound
var ErrNoRound = errors.New("no current round found")

var DefaultTimeout = 30 * time.Second
var DefaultRoundWaitTimeout = 5 * time.Second

// Client represents a Tupelo client for interacting with and
// listening to ChainTree events
type Client struct {
	Group        *types.NotaryGroup
	logger       logging.EventLogger
	subscriber   *roundSubscriber
	pubsub       pubsubinterfaces.Pubsubber
	store        nodestore.DagStore
	onStartHooks []OnStartHook

	validators  []chaintree.BlockValidatorFunc
	transactors map[transactions.Transaction_Type]chaintree.TransactorFunc
}

// New instantiates a Client specific to a ChainTree/NotaryGroup. The store should probably be a bitswap peer.
// The store definitely needs access to the round confirmation, checkpoints, etc
func New(opts ...interface{}) *Client {
	backwardCompatibleOpts := make([]Option, len(opts))

	for k, opt := range opts {
		switch v := opt.(type) {
		case Option:
			backwardCompatibleOpts[k] = v
		case *types.NotaryGroup:
			backwardCompatibleOpts[k] = WithNotaryGroup(v)
		case pubsubinterfaces.Pubsubber:
			backwardCompatibleOpts[k] = WithPubsub(v)
		case nodestore.DagStore:
			backwardCompatibleOpts[k] = WithDagStore(v)
		default:
			panic(fmt.Sprintf("Unsupported option to client.New, must be client.Option"))
		}
	}

	config := &Config{}
	err := config.ApplyOptions(backwardCompatibleOpts...)
	if err != nil {
		panic(err)
	}

	client, err := NewWithConfig(context.TODO(), config)
	if err != nil {
		panic(err)
	}

	return client
}

func NewWithConfig(ctx context.Context, config *Config) (*Client, error) {
	err := config.SetDefaults(ctx)
	if err != nil {
		return nil, err
	}
	return &Client{
		Group:        config.Group,
		logger:       config.Logger,
		pubsub:       config.Pubsub,
		store:        config.Store,
		validators:   config.Validators,
		transactors:  config.Transactors,
		subscriber:   config.subscriber,
		onStartHooks: config.OnStartHooks,
	}, nil
}

func (c *Client) DagStore() nodestore.DagStore {
	return c.store
}

func (c *Client) Start(ctx context.Context) error {
	for _, hook := range c.onStartHooks {
		err := hook(c)
		if err != nil {
			return err
		}
	}

	err := c.subscriber.start(ctx)
	if err != nil {
		return fmt.Errorf("error subscribing: %w", err)
	}

	return nil
}

func (c *Client) PlayTransactions(ctx context.Context, tree *consensus.SignedChainTree, treeKey *ecdsa.PrivateKey, transactions []*transactions.Transaction) (*gossip.Proof, error) {
	abr, err := c.NewAddBlockRequest(ctx, tree, treeKey, transactions)
	if err != nil {
		return nil, fmt.Errorf("error creating NewAddBlockRequest: %w", err)
	}
	proof, err := c.Send(ctx, abr, DefaultTimeout)
	if err != nil {
		return nil, fmt.Errorf("error sending tx: %w", err)
	}

	tipCid, err := cid.Cast(proof.Tip)
	if err != nil {
		return nil, fmt.Errorf("error casting tip cid: %v", err)
	}
	newChainTree, err := chaintree.NewChainTree(ctx, dag.NewDag(ctx, tipCid, tree.ChainTree.Dag.Store), tree.ChainTree.BlockValidators, tree.ChainTree.Transactors)
	if err != nil {
		return nil, fmt.Errorf("error creating new tree: %w", err)
	}
	tree.ChainTree = newChainTree
	tree.Proof = proof
	return proof, nil
}

func (c *Client) GetLatest(parentCtx context.Context, did string) (*consensus.SignedChainTree, error) {
	sp, ctx := opentracing.StartSpanFromContext(parentCtx, "client.GetLatest")
	defer sp.Finish()

	sw := &safewrap.SafeWrap{}

	c.logger.Debugf("getting tip for latest")

	proof, err := c.GetTip(ctx, did)
	if (err == ErrNotFound) || (err == ErrNoRound) {
		return nil, err
	}
	if err != nil {
		return nil, fmt.Errorf("error getting tip: %w", err)
	}

	abr := proof.AddBlockRequest

	// cast the new and previous tips from the ABR

	newTip, err := cid.Cast(abr.NewTip)
	if err != nil {
		return nil, fmt.Errorf("error casting newTip: %w", err)
	}

	previousTip, err := cid.Cast(abr.PreviousTip)
	if err != nil {
		return nil, fmt.Errorf("error casting previousTip: %w", err)
	}

	// now we save all the state blocks into the store

	cborNodes := make([]format.Node, len(abr.State))
	for i, node := range abr.State {
		cborNodes[i] = sw.Decode(node)
	}
	if sw.Err != nil {
		return nil, fmt.Errorf("error decoding nodes: %w", sw.Err)
	}

	err = c.store.AddMany(ctx, cborNodes)
	if err != nil {
		return nil, fmt.Errorf("error adding nodes: %w", err)
	}

	// and get the block with headers

	blockWithHeaders := &chaintree.BlockWithHeaders{}
	err = cbornode.DecodeInto(abr.Payload, blockWithHeaders)
	if err != nil {
		return nil, fmt.Errorf("error decoding payload; %w", err)
	}

	// first we're going to create a chaintree at the *previous* tip and then we're going to play the new
	// BlockWithHeaders on there so that the *new* state blocks get added to the local store

	var treeDag *dag.Dag
	if abr.Height > 0 {
		treeDag = dag.NewDag(ctx, previousTip, c.store)
	} else {
		treeDag = consensus.NewEmptyTree(ctx, string(abr.ObjectId), c.store)
	}

	tree, err := chaintree.NewChainTree(ctx, treeDag, c.validators, c.transactors)
	if err != nil {
		return nil, fmt.Errorf("error creating new tree: %w", err)
	}

	c.logger.Debugf("process block immutable")

	newTree, valid, err := tree.ProcessBlockImmutable(ctx, blockWithHeaders)
	if !valid || err != nil {
		return nil, fmt.Errorf("error processing block: %w", err)
	}

	// sanity check the new tip
	if !newTree.Dag.Tip.Equals(newTip) {
		return nil, fmt.Errorf("error, tips did not match %s != %s", newTree.Dag.Tip.String(), newTip.String())
	}

	// return our signed chaintree with all the new blocks already in the client store
	return &consensus.SignedChainTree{
		ChainTree: newTree,
		Proof:     proof,
	}, nil
}

func (c *Client) CurrentRound() *types.RoundConfirmationWrapper {
	return c.subscriber.Current()
}

func (c *Client) WaitForFirstRound(ctx context.Context, timeout time.Duration) error {
	current := c.subscriber.Current()
	if current != nil {
		return nil
	}

	ch := make(chan *types.RoundConfirmationWrapper, 1)
	defer close(ch)

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	sub, err := c.SubscribeToRounds(context.TODO(), ch)
	if err != nil {
		return fmt.Errorf("error subscribing: %w", err)
	}
	defer c.UnsubscribeFromRounds(sub)

	doneCh := ctx.Done()

	select {
	case <-ch:
		return nil
	case <-timer.C:
		return ErrNoRound
	case <-doneCh:
		c.logger.Warning("ctx closed before WaitForFirstRound")
		return nil
	}
}

func (c *Client) GetTip(ctx context.Context, did string) (*gossip.Proof, error) {
	err := c.WaitForFirstRound(ctx, DefaultRoundWaitTimeout)
	if err != nil {
		return nil, err
	}
	confirmation := c.subscriber.Current()
	if confirmation == nil {
		return nil, ErrNoRound
	}
	currentRound, err := confirmation.FetchCompletedRound(ctx)
	if err != nil {
		return nil, fmt.Errorf("error fetching round: %w", err)
	}
	hamtNode, err := currentRound.FetchHamt(ctx)
	if err != nil {
		return nil, fmt.Errorf("error fetching hamt: %w", err)
	}

	txCID := &cid.Cid{}
	err = hamtNode.Find(ctx, did, txCID)
	if err != nil {
		if err == hamt.ErrNotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("error fetching tip: %w", err)
	}

	abrNode, err := c.store.Get(ctx, *txCID)
	if err != nil {
		return nil, fmt.Errorf("error getting ABR: %w", err)
	}
	abr := &services.AddBlockRequest{}
	err = cbornode.DecodeInto(abrNode.RawData(), abr)
	if err != nil {
		return nil, fmt.Errorf("error decoding ABR: %w", err)
	}

	return &gossip.Proof{
		ObjectId:          []byte(did),
		RoundConfirmation: confirmation.Value(),
		Tip:               abr.NewTip,
		AddBlockRequest:   abr,
	}, nil
}

func (c *Client) Send(ctx context.Context, abr *services.AddBlockRequest, timeout time.Duration) (*gossip.Proof, error) {
	resp := make(chan *gossip.Proof)
	defer close(resp)

	sub, err := c.SubscribeToAbr(ctx, abr, resp)
	if err != nil {
		return nil, err
	}
	defer c.UnsubscribeFromAbr(sub)

	if err := c.SendWithoutWait(ctx, abr); err != nil {
		return nil, fmt.Errorf("error sending Tx: %w", err)
	}

	ticker := time.NewTimer(timeout)
	defer ticker.Stop()

	select {
	case proof := <-resp:
		return proof, nil
	case <-ticker.C:
		return nil, ErrTimeout
	}
}

func (c *Client) SendWithoutWait(ctx context.Context, abr *services.AddBlockRequest) error {
	bits, err := abr.Marshal()
	if err != nil {
		return fmt.Errorf("error marshaling: %w", err)
	}

	err = c.pubsub.Publish(c.Group.Config().TransactionTopic, bits)
	if err != nil {
		return fmt.Errorf("error publishing: %w", err)
	}

	return nil
}

func (c *Client) SubscribeToRounds(ctx context.Context, ch chan *types.RoundConfirmationWrapper) (subscription, error) {
	doneCh := ctx.Done()
	var sub subscription
	sub = c.subscriber.stream.Subscribe(func(evt interface{}) {
		select {
		case ch <- evt.(*ValidationNotification).RoundConfirmation:
		case <-doneCh:
			c.subscriber.unsubscribe(sub)
		default:
			c.logger.Debug("round subscriber could not put to channel")
		}
	})
	return sub, nil
}

func (c *Client) UnsubscribeFromRounds(s subscription) {
	c.subscriber.unsubscribe(s)
}

func (c *Client) SubscribeToAbr(ctx context.Context, abr *services.AddBlockRequest, ch chan *gossip.Proof) (subscription, error) {
	return c.subscriber.subscribe(ctx, abr, ch)
}

func (c *Client) UnsubscribeFromAbr(s subscription) {
	c.subscriber.unsubscribe(s)
}

func (c *Client) SubscribeToDid(ctx context.Context, did string, ch chan *gossip.Proof) error {
	confirmationCh := make(chan *types.RoundConfirmationWrapper, 10)
	_, err := c.SubscribeToRounds(ctx, confirmationCh)
	if err != nil {
		return fmt.Errorf("error subscribing %w", err)
	}

	var currentProof *gossip.Proof

	doneCh := ctx.Done()
	go func() {
	ChannelLoop:
		for {
			select {
			case <-doneCh:
				break ChannelLoop
			case <-confirmationCh:
				latestProof, err := c.GetTip(ctx, did)
				if err != nil {
					c.logger.Errorf("error getting tip: %w", err)
					break ChannelLoop // for now just return here - maybe in the future we want to do retries
				}

				if currentProof == nil || !bytes.Equal(currentProof.Tip, latestProof.Tip) {
					ch <- latestProof
					currentProof = latestProof
				}
			}
		}
		// signal to the outer channel that the subscription has terminated
		ch <- nil
	}()

	return nil
}
