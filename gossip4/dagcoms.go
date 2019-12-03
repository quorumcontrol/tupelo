package gossip4

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	lru "github.com/hashicorp/golang-lru"
	cbornode "github.com/ipfs/go-ipld-cbor"

	"github.com/AsynkronIT/protoactor-go/actor"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-msgio"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
)

var genesis cid.Cid

const gossip4Protocol = "tupelo/v0.0.1"

const transactionTopic = "g4-transactions"

type mempool map[cid.Cid]*services.AddBlockRequest

func (m mempool) Keys() []cid.Cid {
	keys := make([]cid.Cid, len(m))
	i := 0
	for k := range m {
		keys[i] = k
		i++
	}
	return keys
}

type roundHolder map[uint64]*round

func init() {
	cbornode.RegisterCborType(services.AddBlockRequest{})
	sw := &safewrap.SafeWrap{}
	genesis = sw.WrapObject("genesis").Cid()
	if sw.Err != nil {
		panic(fmt.Errorf("error setting up genesis: %w", sw.Err))
	}
}

type snowballerDone struct {
	err error
}

type Node struct {
	p2pNode     p2p.Node
	signKey     *bls.SignKey
	notaryGroup *types.NotaryGroup
	dagStore    nodestore.DagStore
	hamtStore   *hamt.CborIpldStore
	pubsub      *pubsub.PubSub

	mempool  mempool
	inflight *lru.Cache

	logger logging.EventLogger

	rounds       roundHolder
	currentRound uint64

	snowballer *snowballer

	signerIndex int

	pid         *actor.PID
	snowballPid *actor.PID
}

type NewNodeOptions struct {
	P2PNode        p2p.Node
	SignKey        *bls.SignKey
	NotaryGroup    *types.NotaryGroup
	DagStore       nodestore.DagStore
	Logger         logging.EventLogger
	PreviousRounds roundHolder
	CurrentRound   uint64
}

func NewNode(ctx context.Context, opts *NewNodeOptions) (*Node, error) {
	hamtStore := dagStoreToCborIpld(opts.DagStore)

	var signerIndex int
	for i, s := range opts.NotaryGroup.AllSigners() {
		if bytes.Equal(s.VerKey.Bytes(), opts.SignKey.MustVerKey().Bytes()) {
			signerIndex = i
			break
		}
	}

	logger := logging.Logger(fmt.Sprintf("node-%d", signerIndex))

	logger.Debugf("signerIndex: %d", signerIndex)

	cache, err := lru.New(500)
	if err != nil {
		return nil, fmt.Errorf("error creating cache: %w", err)
	}

	if opts.PreviousRounds == nil {
		opts.PreviousRounds = make(roundHolder)
	}

	r, ok := opts.PreviousRounds[opts.CurrentRound]
	if !ok {
		r = newRound(opts.CurrentRound)
		opts.PreviousRounds[opts.CurrentRound] = r
	}

	networkedSnowball := &snowballer{
		snowball: r.snowball,
		height:   r.height,
		host:     opts.P2PNode,
		group:    opts.NotaryGroup,
		logger:   logger,
	}

	return &Node{
		p2pNode:      opts.P2PNode,
		signKey:      opts.SignKey,
		notaryGroup:  opts.NotaryGroup,
		dagStore:     opts.DagStore,
		hamtStore:    hamtStore,
		rounds:       opts.PreviousRounds,
		currentRound: opts.CurrentRound,
		signerIndex:  signerIndex,
		logger:       logger,
		inflight:     cache,
		snowballer:   networkedSnowball,
		mempool:      make(mempool),
	}, nil
}

// func loadNode(ctx context.Context, store *hamt.CborIpldStore, id cid.Cid) (*hamt.Node, error) {
// 	return hamt.LoadNode(ctx, store, id, hamt.UseTreeBitWidth(5))
// }

func (n *Node) Start(ctx context.Context) error {
	pid := actor.EmptyRootContext.Spawn(actor.PropsFromFunc(n.Receive))
	n.pid = pid

	n.snowballPid = actor.EmptyRootContext.Spawn(actor.PropsFromFunc(n.SnowBallReceive))

	go func() {
		<-ctx.Done()
		n.logger.Debugf("node stopped")
		actor.EmptyRootContext.Poison(n.pid)
		actor.EmptyRootContext.Poison(n.snowballPid)
	}()

	validator, err := newTransactionValidator(n.logger, n.notaryGroup, pid)
	if err != nil {
		return fmt.Errorf("error setting up: %v", err)
	}

	n.pubsub = n.p2pNode.GetPubSub()

	err = n.pubsub.RegisterTopicValidator(transactionTopic, validator.validate)
	if err != nil {
		return fmt.Errorf("error registering topic validator: %v", err)
	}

	sub, err := n.pubsub.Subscribe(transactionTopic)
	if err != nil {
		return fmt.Errorf("error subscribing %v", err)
	}

	n.p2pNode.SetStreamHandler(gossip4Protocol, func(s network.Stream) {
		actor.EmptyRootContext.Send(n.snowballPid, s)
	})

	// don't do anything with these messages because we actually get them
	// fully decoded in the actor spun up above
	go func() {
		for {
			_, err := sub.Next(ctx)
			if err != nil {
				n.logger.Warningf("error getting sub message: %v", err)
				return
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		done := make(chan error, 1)
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				if !n.snowballer.Started() && len(n.mempool) > 0 {
					n.logger.Debugf("starting snowballer and preferring %v", n.mempool.Keys())
					done = make(chan error, 1)
					n.snowballer.snowball.Prefer(&Vote{
						Block: &Block{
							Height:       n.currentRound,
							Transactions: n.mempool.Keys(),
						},
					})
					n.snowballer.start(ctx, done)
				}
			case err := <-done:
				actor.EmptyRootContext.Send(n.snowballPid, &snowballerDone{err: err})
			}
		}
	}()

	n.logger.Debugf("node starting")

	return nil
}

func (n *Node) Receive(actorContext actor.Context) {
	switch msg := actorContext.Message().(type) {
	case *services.AddBlockRequest:
		n.handleAddBlockRequest(actorContext, msg)
	}
}

func (n *Node) SnowBallReceive(actorContext actor.Context) {
	switch msg := actorContext.Message().(type) {
	case network.Stream:
		go func() {
			n.handleStream(msg)
		}()
	case *snowballerDone:
		n.logger.Infof("round %d decided with err: %v", n.currentRound, msg.err)
	}
}

func (n *Node) handleStream(s network.Stream) {
	n.logger.Debugf("handling stream from")
	s.SetWriteDeadline(time.Now().Add(2 * time.Second))
	s.SetReadDeadline(time.Now().Add(1 * time.Second))
	reader := msgio.NewVarintReader(s)
	writer := msgio.NewVarintWriter(s)

	bits, err := reader.ReadMsg()
	if err != nil {
		if err != io.EOF {
			_ = s.Reset()
		}
		n.logger.Warningf("error reading from incoming stream: %v", err)
		return
	}
	//TODO: I don't actually know what this does :)
	reader.ReleaseMsg(bits)

	var height uint64
	err = cbornode.DecodeInto(bits, &height)
	if err != nil {
		n.logger.Warningf("error decoding incoming height: %v", err)
		s.Close()
		return
	}
	n.logger.Debugf("remote looking for height: %d", height)
	// TODO: this is a concurrent read and so needs to be locked, ignoring for now
	r, ok := n.rounds[height]
	response := Block{Height: height}
	if ok {
		n.logger.Debugf("existing round %d", height)
		preferred := r.snowball.Preferred()
		if preferred != nil {
			n.logger.Debugf("existing preferred; %v", preferred)
			response = *preferred.Block
		}
	}
	sw := &safewrap.SafeWrap{}
	wrapped := sw.WrapObject(response)
	if sw.Err != nil {
		n.logger.Errorf("error wrapping: %v", err)
		s.Close()
		return
	}
	err = writer.WriteMsg(wrapped.RawData())
	if err != nil {
		n.logger.Warningf("error writing: %v", err)
		s.Close()
		return
	}

	return
}

func (n *Node) handleAddBlockRequest(actorContext actor.Context, abr *services.AddBlockRequest) {
	ctx, cancel := context.WithCancel(context.TODO())
	defer cancel()

	n.logger.Debugf("handling message: ObjectId: %s, Height: %d", abr.ObjectId, abr.Height)
	// look into the hamt, and get the current ABR
	// if this is the next height then replace it
	// if this is older drop it
	// if this is in the future, keep it in flight

	current, err := n.getCurrent(ctx, string(abr.ObjectId))
	if err != nil {
		n.logger.Errorf("error getting current: %v", err)
		return
	}

	// if the current is nil, then this ABR is acceptable if its height is 0
	if current == nil {
		if abr.Height == 0 {
			err = n.storeAbr(ctx, abr)
			if err != nil {
				n.logger.Errorf("error getting current: %v", err)
				return
			}
			// err = n.handlePostSave(ctx, actorContext, abr, current)
			// if err != nil {
			// 	n.logger.Errorf("error handling postSave: %v", err)
			// }
			return
		}
		n.storeAsInFlight(ctx, abr)
		return
	}

	// if this msg height is lower than current then just drop it
	if current.Height > abr.Height {
		return
	}

	// Is this the next height ABR?
	if current.Height+1 == abr.Height {
		// then this is the next height, let's save it if the tips match

		if !bytes.Equal(current.PreviousTip, abr.PreviousTip) {
			n.logger.Warningf("tips did not match")
			return
		}

		err = n.storeAbr(ctx, abr)
		if err != nil {
			n.logger.Errorf("error getting current: %v", err)
			return
		}
		// err = n.handlePostSave(ctx, actorContext, abr, current)
		// if err != nil {
		// 	n.logger.Errorf("error handling postSave: %v", err)
		// }
		return
	}

	if abr.Height > current.Height+1 {
		// this is in the future so just queue it up
		n.storeAsInFlight(ctx, abr)
		return
	}

	// TODO: handle byzantine case of msg.Height == current.Height
}

func (n *Node) storeAsInFlight(ctx context.Context, abr *services.AddBlockRequest) {
	n.logger.Debugf("storing in inflight %s height: %d", string(abr.ObjectId), abr.Height)
	n.inflight.Add(inFlightID(abr.ObjectId, abr.Height), abr)
}

func inFlightID(objectID []byte, height uint64) string {
	return string(objectID) + strconv.FormatUint(height, 10)
}

func (n *Node) storeAbr(ctx context.Context, abr *services.AddBlockRequest) error {
	sw := &safewrap.SafeWrap{}
	wrapped := sw.WrapObject(abr)
	if sw.Err != nil {
		return sw.Err
	}
	n.logger.Debugf("storing in mempool %s", wrapped.Cid().String())
	n.mempool[wrapped.Cid()] = abr

	nextKey := inFlightID(abr.ObjectId, abr.Height+1)
	// if the next height Tx is here we can also queue that up
	next, ok := n.inflight.Get(nextKey)
	if ok {
		nextAbr := next.(*services.AddBlockRequest)
		if bytes.Equal(nextAbr.PreviousTip, abr.NewTip) {
			return n.storeAbr(ctx, nextAbr)
		}
		// This is commented out because I'm not sure exactly what we want to do here
		// if there is a big tree of Txs that aren't going to make it in, we don't necessarily
		// want to throw them away because we want the other nodes to know we have them.
		// // otherwise we can just throw that one away
		// n.inflight.Remove(nextKey)
	}

	return nil
}

func (n *Node) getCurrent(ctx context.Context, objectID string) (*services.AddBlockRequest, error) {
	if n.currentRound == 0 {
		return nil, nil // this is the genesis round: we, by definition, have no state
	}

	var abrCid cid.Cid
	var abr *services.AddBlockRequest

	lockedRound, ok := n.rounds[n.currentRound-1]
	if !ok {
		return nil, fmt.Errorf("we don't have the previous round")
	}

	err := lockedRound.state.Find(ctx, objectID, &abrCid)
	if err != nil {
		if err == hamt.ErrNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting abrCID: %v", err)
	}

	if !abrCid.Equals(cid.Undef) {
		err = n.hamtStore.Get(ctx, abrCid, abr)
		if err != nil {
			return nil, fmt.Errorf("error getting abr: %v", err)
		}
	}

	return abr, nil
}
