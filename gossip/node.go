package gossip

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"time"

	lru "github.com/hashicorp/golang-lru"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/opentracing/opentracing-go"

	"github.com/AsynkronIT/protoactor-go/actor"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	msgio "github.com/libp2p/go-msgio"

	cid "github.com/ipfs/go-cid"
	hamt "github.com/ipfs/go-hamt-ipld"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/messages/v2/build/go/gossip"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/hamtwrapper"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/types"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
	sigutils "github.com/quorumcontrol/tupelo-go-sdk/signatures"
)

const gossipProtocol = "tupelo/v0.0.1"

func init() {
	cbornode.RegisterCborType(services.AddBlockRequest{})
}

type snowballTicker struct {
	ctx context.Context
}

type startSnowball struct {
	ctx context.Context
}

type Node struct {
	name string

	p2pNode     p2p.Node
	signKey     *bls.SignKey
	notaryGroup *types.NotaryGroup
	dagStore    nodestore.DagStore
	hamtStore   *hamt.CborIpldStore
	pubsub      *pubsub.PubSub

	mempool  *mempool
	inflight *lru.Cache

	logger logging.EventLogger

	rounds *roundHolder

	snowballer *snowballer
	validator  *TransactionValidator

	signerIndex int

	pid         *actor.PID
	snowballPid *actor.PID
	syncerPid   *actor.PID

	rootContext *actor.RootContext

	startCtx context.Context

	closer io.Closer
}

type NewNodeOptions struct {
	P2PNode          p2p.Node
	SignKey          *bls.SignKey
	NotaryGroup      *types.NotaryGroup
	DagStore         nodestore.DagStore
	Name             string             // optional
	RootActorContext *actor.RootContext // optional
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func NewNode(ctx context.Context, opts *NewNodeOptions) (*Node, error) {
	hamtStore := hamtwrapper.DagStoreToCborIpld(opts.DagStore)

	var signerIndex int
	for i, s := range opts.NotaryGroup.AllSigners() {
		if bytes.Equal(s.VerKey.Bytes(), opts.SignKey.MustVerKey().Bytes()) {
			signerIndex = i
			break
		}
	}

	cache, err := lru.New(500)
	if err != nil {
		return nil, fmt.Errorf("error creating cache: %w", err)
	}

	// TODO: we need a way to bootstrap a node coming back up and not in the genesis state.
	// shouldn't be too hard, just save the latest round into a key/value store somewhere
	// and then it can come back up and bootstrap itself up to the latest in the network
	holder := newRoundHolder()
	r := newRound(0, 0, 0, min(defaultK, int(opts.NotaryGroup.Size())-1))
	holder.SetCurrent(r)

	n := &Node{
		name:        opts.Name,
		p2pNode:     opts.P2PNode,
		signKey:     opts.SignKey,
		notaryGroup: opts.NotaryGroup,
		dagStore:    opts.DagStore,
		hamtStore:   hamtStore,
		rounds:      holder,
		signerIndex: signerIndex,
		inflight:    cache,
		mempool:     newMempool(),
		rootContext: opts.RootActorContext,
	}

	if n.rootContext == nil {
		n.rootContext = actor.EmptyRootContext
	}

	if n.name == "" {
		n.name = fmt.Sprintf("node-%d", signerIndex)
	}

	logger := logging.Logger(n.name)
	logger.Debugf("signerIndex: %d", signerIndex)
	n.logger = logger

	networkedSnowball := newSnowballer(n, r.height, r.snowball)
	n.snowballer = networkedSnowball
	return n, nil
}

func (n *Node) Start(ctx context.Context) error {
	pid, err := n.rootContext.SpawnNamed(actor.PropsFromFunc(n.Receive), n.name)
	if err != nil {
		return fmt.Errorf("error starting actor: %w", err)
	}
	n.pid = pid

	n.logger.Debugf("pid: %s", pid.Id)

	go func() {
		<-ctx.Done()
		n.logger.Infof("node stopped")
		n.rootContext.Poison(n.pid)
	}()

	validator, err := NewTransactionValidator(ctx, n.logger, n.notaryGroup, pid)
	if err != nil {
		return fmt.Errorf("error setting up: %v", err)
	}
	n.validator = validator

	n.pubsub = n.p2pNode.GetPubSub()

	err = n.pubsub.RegisterTopicValidator(n.notaryGroup.Config().TransactionTopic, validator.validate)
	if err != nil {
		return fmt.Errorf("error registering topic validator: %v", err)
	}

	sub, err := n.pubsub.Subscribe(n.notaryGroup.Config().TransactionTopic)
	if err != nil {
		return fmt.Errorf("error subscribing %v", err)
	}

	n.startCtx = ctx

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
		// every once in a while check to see if the mempool has entries and if the
		// snowballer has started and start the snowballer if it isn't
		// this is handled by the snowball actor spun up in the node's root actor (see: Receive function)
		ticker := time.NewTicker(200 * time.Millisecond)
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				n.rootContext.Send(n.pid, &snowballTicker{ctx: ctx})
			}
		}
	}()

	n.logger.Debugf("node starting")

	return nil
}

func (n *Node) Bootstrap(ctx context.Context, bootstrapAddrs []string) error {
	closer, err := n.p2pNode.Bootstrap(bootstrapAddrs)
	if err != nil {
		return fmt.Errorf("error bootstrapping gossip node: %v", err)
	}

	n.closer = closer

	return n.p2pNode.WaitForBootstrap(1, 10*time.Second)
}

func (n *Node) Close() error {
	err := n.closer.Close()
	if err != nil {
		return fmt.Errorf("error closing node IO: %v", err)
	}

	return nil
}

func (n *Node) setupSnowball(actorContext actor.Context) {
	snowballPid, err := actorContext.SpawnNamed(actor.PropsFromFunc(n.SnowBallReceive), "snowballReceiver")
	if err != nil {
		panic(err)
	}
	n.snowballPid = snowballPid
	n.p2pNode.SetStreamHandler(gossipProtocol, func(s network.Stream) {
		n.rootContext.Send(snowballPid, s)
	})
}

func (n *Node) setupSyncer(actorContext actor.Context) {
	syncerPid, err := actorContext.SpawnNamed(actor.PropsFromProducer(func() actor.Actor {
		return &transactionGetter{
			nodeActor: actorContext.Self(),
			logger:    n.logger,
			store:     n.hamtStore,
			validator: n.validator,
		}
	}), "transactionsyncer")
	if err != nil {
		panic(err)
	}
	n.syncerPid = syncerPid
}

func (n *Node) Receive(actorContext actor.Context) {
	switch msg := actorContext.Message().(type) {
	case *actor.Started:
		n.setupSyncer(actorContext)
		n.setupSnowball(actorContext)
	case *AddBlockWrapper:
		n.handleAddBlockRequest(actorContext, msg)
	case *snowballTicker:
		if !n.snowballer.Started() && n.mempool.Length() > 0 {
			actorContext.Send(n.snowballPid, &startSnowball{ctx: msg.ctx})
		}
	case *snowballerDone:
		n.handleSnowballerDone(msg)
	default:
		n.logger.Debugf("root node actor received other %T message: %+v", msg, msg)
	}
}

func (n *Node) confirmCompletedRound(ctx context.Context, completedRound *types.RoundWrapper) (*types.RoundConfirmationWrapper, error) {
	roundCid := completedRound.CID()

	sig, err := sigutils.BLSSign(ctx, n.signKey, roundCid.Bytes(), len(n.notaryGroup.Signers), n.signerIndex)
	if err != nil {
		return nil, fmt.Errorf("error signing current state checkpoint: %v", err)
	}

	conf := &gossip.RoundConfirmation{
		RoundCid:  roundCid.Bytes(),
		Signature: sig,
		Height:    completedRound.Height(),
	}
	return types.WrapRoundConfirmation(conf), nil
}

func (n *Node) publishCompletedRound(ctx context.Context) error {
	current := n.rounds.Current()
	currentStateCid, err := n.hamtStore.Put(ctx, current.state)
	if err != nil {
		return fmt.Errorf("error getting current state cid: %v", err)
	}

	if !current.snowball.Decided() {
		return fmt.Errorf("can't publish an undecided round")
	}

	err = n.dagStore.Add(ctx, current.snowball.Preferred().Checkpoint.Wrapped())
	if err != nil {
		return fmt.Errorf("error adding to dag store: %w", err)
	}

	preferredCheckpointCid := current.snowball.Preferred().Checkpoint.CID()

	completedRound := &gossip.Round{
		Height:        current.height,
		StateCid:      currentStateCid.Bytes(),
		CheckpointCid: preferredCheckpointCid.Bytes(),
	}

	wrappedCompletedRound := types.WrapRound(completedRound)

	err = n.dagStore.Add(ctx, wrappedCompletedRound.Wrapped())
	if err != nil {
		return fmt.Errorf("error adding to dag store: %w", err)
	}

	conf, err := n.confirmCompletedRound(ctx, wrappedCompletedRound)
	if err != nil {
		return fmt.Errorf("error confirming completed round: %v", err)
	}

	n.logger.Debugf("publishing round confirmed to: %s", n.notaryGroup.ID)

	return n.pubsub.Publish(n.notaryGroup.ID, conf.Data())
}

func (n *Node) handleSnowballerDone(msg *snowballerDone) {
	sp := opentracing.StartSpan("gossip4.round.done")
	defer sp.Finish()

	if msg.err != nil {
		n.logger.Errorf("snowballer crashed: %v", msg.err)
		// TODO: How should we recover from this?
		return
	}

	preferred := n.snowballer.snowball.Preferred()

	completedRound := n.rounds.Current()

	n.logger.Infof("round %d decided with err: %v: %s (len: %d)", completedRound.height, msg.err, preferred.ID(), len(preferred.Checkpoint.AddBlockRequests()))
	n.logger.Debugf("round %d transactions %v", completedRound.height, preferred.Checkpoint.AddBlockRequests)
	// take all the transactions from the decided round, remove them from the mempool and apply them to the state
	// increase the currentRound and create a new Round in the roundHolder
	// state updating should be more robust here to make sure transactions don't stomp on each other and can probably happen in the background
	// probably need to recheck what's left in the mempool too for now we're ignoring all that as a PoC

	rootNodeSp := opentracing.StartSpan("gossip4.getRoot", opentracing.ChildOf(sp.Context()))

	var rootNode *hamt.Node
	if completedRound.height == 0 {
		rootNode = hamt.NewNode(n.hamtStore, hamt.UseTreeBitWidth(5))
	} else {
		previousRound, _ := n.rounds.Get(completedRound.height - 1)
		n.logger.Debugf("previous state for %d: %v", completedRound.height-1, previousRound.state)
		rootNode = previousRound.state.Copy()
	}
	n.logger.Debugf("current round: %d, node: %v", completedRound, rootNode)
	rootNodeSp.Finish()

	processSp := opentracing.StartSpan("gossip4.processTxs", opentracing.ChildOf(sp.Context()))
	for _, cidBytes := range preferred.Checkpoint.AddBlockRequests() {
		txSp := opentracing.StartSpan("gossip4.tx", opentracing.ChildOf(processSp.Context()))
		abrCid, err := cid.Cast(cidBytes)
		if err != nil {
			panic(fmt.Errorf("error casting add block request cid: %v", err))
		}

		abrWrapper := n.mempool.Get(abrCid)
		abrWrapper.SetTag("confirmed", true)

		abr := abrWrapper.AddBlockRequest
		if abrWrapper == nil {
			n.logger.Errorf("I DO NOT HAVE THE TRANSACTION: %s", abrCid.String())
			panic("an accepted checkpoint should not have a Tx we don't know about")
		}

		setSp := opentracing.StartSpan("gossip4.tx.set", opentracing.ChildOf(processSp.Context()))

		err = rootNode.Set(msg.ctx, string(abr.ObjectId), abrCid)
		if err != nil {
			panic(fmt.Errorf("error setting hamt: %w", err))
		}
		setSp.Finish()

		// mempool calls StopTrace on our abrWrapper
		n.mempool.DeleteIDAndConflictSet(abrCid)
		n.logger.Debugf("looking for %s height: %d", abr.ObjectId, abr.Height+1)
		// if we have the next update in our inflight, we can queue that up here
		nextKey := inFlightID(abr.ObjectId, abr.Height+1)
		// if the next height Tx is here we can also queue that up
		next, ok := n.inflight.Get(nextKey)
		if ok {
			n.logger.Debugf("found %s height: %d in inflight", abr.ObjectId, abr.Height+1)
			nextAbrWrapper := next.(*AddBlockWrapper)
			if bytes.Equal(nextAbrWrapper.AddBlockRequest.PreviousTip, abrWrapper.AddBlockRequest.NewTip) {
				if err := n.storeAbr(msg.ctx, nextAbrWrapper); err != nil {
					n.logger.Warningf("error storing abr: %v", err)
				}
			}
			n.inflight.Remove(nextKey)
		}
		txSp.Finish()
	}
	processSp.Finish()

	flushSp := opentracing.StartSpan("gossip4.flush", opentracing.ChildOf(sp.Context()))
	err := rootNode.Flush(msg.ctx)
	if err != nil {
		panic(fmt.Errorf("error flushing rootNode: %w", err))
	}
	flushSp.Finish()

	n.logger.Debugf("setting round at %d to rootNode: %v", completedRound.height, rootNode)
	completedRound.state = rootNode
	n.logger.Debugf("after setting: %v", completedRound.state)

	publishSp := opentracing.StartSpan("gossip4.flush", opentracing.ChildOf(sp.Context()))
	// Notify clients of the new checkpoint
	err = n.publishCompletedRound(context.TODO())
	if err != nil {
		n.logger.Errorf("error publishing current round: %v", err)
	}
	publishSp.Finish()

	round := newRound(completedRound.height+1, 0, 0, min(defaultK, int(n.notaryGroup.Size())-1))
	n.rounds.SetCurrent(round)
	n.snowballer = newSnowballer(n, round.height, round.snowball)
}

func (n *Node) SnowBallReceive(actorContext actor.Context) {
	switch msg := actorContext.Message().(type) {
	case network.Stream:
		go func() {
			n.handleStream(actorContext, msg)
		}()
	case *startSnowball:
		n.snowballer.Start(msg.ctx)
	default:
		n.logger.Debugf("snowball actor received other %T message: %+v", msg, msg)
	}
}

func (n *Node) handleStream(actorContext actor.Context, s network.Stream) {
	// n.logger.Debugf("handling stream from")
	if err := s.SetWriteDeadline(time.Now().Add(2 * time.Second)); err != nil {
		n.logger.Errorf("error setting write deadline: %v", err)
	}
	if err := s.SetReadDeadline(time.Now().Add(1 * time.Second)); err != nil {
		n.logger.Errorf("error setting read deadline: %v", err)
	}

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
	// n.logger.Debugf("remote looking for height: %d", height)

	r, ok := n.rounds.Get(height)
	cp := &gossip.Checkpoint{Height: height}
	response := types.WrapCheckpoint(cp)

	if ok {
		// n.logger.Debugf("existing round %d", height)
		preferred := r.snowball.Preferred()
		if preferred == nil && r.height >= n.rounds.Current().height {
			actorContext.Send(actorContext.Self(), &startSnowball{ctx: n.startCtx})
		} else {
			// n.logger.Debugf("existing preferred; %v", preferred)
			response = preferred.Checkpoint
		}
	}
	wrapped := response.Wrapped()

	err = writer.WriteMsg(wrapped.RawData())
	if err != nil {
		n.logger.Warningf("error writing: %v", err)
		s.Close()
		return
	}
}

func (n *Node) handleAddBlockRequest(actorContext actor.Context, abrWrapper *AddBlockWrapper) {
	abr := abrWrapper.AddBlockRequest
	sp := abrWrapper.NewSpan("gossip4.handleAddBlockRequest")
	defer sp.Finish()

	ctx, cancel := context.WithCancel(abrWrapper.GetContext())
	defer cancel()

	n.logger.Debugf("handling message: ObjectId: %s, Height: %d", abr.ObjectId, abr.Height)
	// look into the hamt, and get the current ABR
	// if this is the next height then replace it
	// if this is older drop it
	// if this is in the future, keep it in flight

	current, err := n.getCurrent(ctx, string(abr.ObjectId))
	if err != nil {
		abrWrapper.SetTag("error", err.Error())
		defer abrWrapper.StopTrace()
		n.logger.Errorf("error getting current: %v", err)
		return
	}

	// if the current is nil, then this ABR is acceptable if its height is 0
	if current == nil {
		if abr.Height == 0 {
			err = n.storeAbr(ctx, abrWrapper)
			if err != nil {
				abrWrapper.SetTag("error", err.Error())
				defer abrWrapper.StopTrace()
				n.logger.Errorf("error getting current: %v", err)
				return
			}
			return
		}
		n.storeAsInFlight(ctx, abrWrapper)
		return
	}

	// if this msg height is lower than current then just drop it
	if current.Height > abr.Height {
		abrWrapper.SetTag("error", "current height is higher than ABR height")
		defer abrWrapper.StopTrace()
		return
	}

	// Is this the next height ABR?
	if current.Height+1 == abr.Height {
		// then this is the next height, let's save it if the tips match

		if !bytes.Equal(current.NewTip, abr.PreviousTip) {
			n.logger.Warningf("tips did not match")
			abrWrapper.SetTag("error", "tips did not match")
			defer abrWrapper.StopTrace()
			return
		}

		err = n.storeAbr(ctx, abrWrapper)
		if err != nil {
			err := fmt.Errorf("error storing abr: %w", err)
			n.logger.Errorf("error storing abr: %v", err)
			abrWrapper.SetTag("error", err.Error())
			defer abrWrapper.StopTrace()
			return
		}

		return
	}

	if abr.Height > current.Height+1 {
		// this is in the future so just queue it up
		n.storeAsInFlight(ctx, abrWrapper)
		return
	}

	if abr.Height == current.Height {
		msg := "byzantine: same height as current"
		abrWrapper.SetTag("error", msg)
		defer abrWrapper.StopTrace()
		n.logger.Errorf("error storing abr: %v", msg)
		return
	}
}

func (n *Node) storeAsInFlight(ctx context.Context, abrWrapper *AddBlockWrapper) {
	abr := abrWrapper.AddBlockRequest
	n.logger.Infof("storing in inflight %s height: %d", string(abr.ObjectId), abr.Height)
	n.inflight.Add(inFlightID(abr.ObjectId, abr.Height), abrWrapper)
}

func inFlightID(objectID []byte, height uint64) string {
	return string(objectID) + strconv.FormatUint(height, 10)
}

func (n *Node) storeAbr(ctx context.Context, abrWrapper *AddBlockWrapper) error {
	id, err := n.hamtStore.Put(ctx, abrWrapper.AddBlockRequest)
	if err != nil {
		return fmt.Errorf("error putting abr: %w", err)
	}

	n.logger.Debugf("storing in mempool %s", id.String())
	n.mempool.Add(id, abrWrapper)

	return nil
}

func (n *Node) getCurrent(ctx context.Context, objectID string) (*services.AddBlockRequest, error) {
	currentRound := n.rounds.Current()
	if currentRound.height == 0 {
		return nil, nil // this is the genesis round: we, by definition, have no state
	}

	var abrCid cid.Cid

	lockedRound, ok := n.rounds.Get(currentRound.height - 1)
	if !ok {
		return nil, fmt.Errorf("we don't have the previous round")
	}

	n.logger.Debugf("previous round: %v", lockedRound)

	err := lockedRound.state.Find(ctx, objectID, &abrCid)
	if err != nil {
		if err == hamt.ErrNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("error getting abrCID: %v", err)
	}

	abr := &services.AddBlockRequest{}

	if !abrCid.Equals(cid.Undef) {
		err = n.hamtStore.Get(ctx, abrCid, abr)
		if err != nil {
			return nil, fmt.Errorf("error getting abr: %v", err)
		}
	}

	return abr, nil
}

func (n *Node) PID() *actor.PID {
	return n.pid
}
