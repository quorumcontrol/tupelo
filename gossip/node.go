package gossip

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	lru "github.com/hashicorp/golang-lru"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-hamt-ipld"
	cbornode "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p-core/network"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/libp2p/go-msgio"
	"github.com/opentracing/opentracing-go"
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
const lastCompletedHeightKey = "/Snowball/LastCompletedHeight"
const completedRoundsKeyRoot = "/Snowball/CompletedRounds"

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
	dataStore   datastore.Batching
	pubsub      *pubsub.PubSub

	mempool  *mempool
	inflight *lru.Cache

	logger logging.EventLogger

	rounds *roundHolder

	snowballer  *snowballer
	validator   *TransactionValidator
	stateStorer *stateStorer

	signerIndex int

	pid            *actor.PID
	snowballPid    *actor.PID
	stateStorerPid *actor.PID
	syncerPid      *actor.PID

	rootContext *actor.RootContext

	startCtx context.Context

	closer io.Closer
}

type NewNodeOptions struct {
	P2PNode          p2p.Node
	SignKey          *bls.SignKey
	NotaryGroup      *types.NotaryGroup
	DagStore         nodestore.DagStore
	Datastore        datastore.Batching
	Name             string             // optional
	RootActorContext *actor.RootContext // optional
}

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func roundKey(height uint64) datastore.Key {
	heightStr := strconv.FormatUint(height, 10)
	keyStr := strings.Join([]string{completedRoundsKeyRoot, heightStr}, "/")
	return datastore.NewKey(keyStr)
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

	nodeName := opts.Name
	if nodeName == "" {
		nodeName = fmt.Sprintf("node-%d", signerIndex)
	}

	logger := logging.Logger(nodeName)
	logger.Debugf("signerIndex: %d", signerIndex)

	dataStore := opts.Datastore
	dagStore := opts.DagStore

	n := &Node{
		name:        nodeName,
		p2pNode:     opts.P2PNode,
		signKey:     opts.SignKey,
		notaryGroup: opts.NotaryGroup,
		dagStore:    dagStore,
		hamtStore:   hamtStore,
		dataStore:   dataStore,
		signerIndex: signerIndex,
		inflight:    cache,
		mempool:     newMempool(),
		rootContext: opts.RootActorContext,
		logger:      logger,
	}

	err = n.initRoundHolder()
	if err != nil {
		return nil, fmt.Errorf("could not init roundHolder: %w", err)
	}

	if n.rootContext == nil {
		n.rootContext = actor.EmptyRootContext
	}

	r := n.rounds.Current()
	networkedSnowball := newSnowballer(n, r.height, r.snowball)
	n.snowballer = networkedSnowball

	n.stateStorer = newStateStorer(logger, n.dagStore)
	n.pubsub = n.p2pNode.GetPubSub()

	return n, nil
}

func (n *Node) initRoundHolder() error {
	rhOpts := &roundHolderOpts{
		DataStore: n.dataStore,
		DagStore:  n.dagStore,
		HamtStore: n.hamtStore,
	}
	holder, err := newRoundHolder(rhOpts)
	if err != nil {
		return fmt.Errorf("could not create roundHolder: %w", err)
	}

	heightBytes, err := n.dataStore.Get(datastore.NewKey(lastCompletedHeightKey))
	if err != nil && err != datastore.ErrNotFound {
		return fmt.Errorf("could not look up last completed round: %w", err)
	}

	var height uint64
	if err == datastore.ErrNotFound {
		height = uint64(0)
	} else {
		var n int
		height, n = binary.Uvarint(heightBytes)
		if n <= 0 {
			return fmt.Errorf("could not decode last completed round height")
		}

		height += 1
	}

	r := newRound(height, 0, 0, min(defaultK, int(n.notaryGroup.Size())-1))
	holder.SetCurrent(r)

	n.logger.Infof("initializing snowball at height: %d with alpha: %f, beta: %d, and k: %d",
		r.height, r.snowball.alpha, r.snowball.beta, r.snowball.k)

	n.rounds = holder

	return nil
}

func (n *Node) Start(ctx context.Context) error {
	n.startCtx = ctx

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

	n.logger.Debugf("node starting")

	return nil
}

func (n *Node) maybeStartSnowball(ctx context.Context) {
	if !n.snowballer.Started() && n.mempool.Length() > 0 {
		n.rootContext.Send(n.snowballPid, &startSnowball{ctx: ctx})
	}
}

// republish is called by a ticker and every so often will send out its signature on the latest round
// if the signer is sitting idle. If a snowball is in progress then we don't republish
func (n *Node) maybeRepublish(ctx context.Context) {
	if n.snowballer.Started() {
		return
	}

	if round := n.rounds.Current(); round != nil {
		previousRound, found := n.rounds.Get(round.height - 1)

		if found && previousRound != nil && previousRound.published {
			n.logger.Debugf("republishing round: %d", previousRound.height)
			n.publishCompletedRound(ctx, previousRound)
		}
	}
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

func (n *Node) setupStateStorer(actorContext actor.Context) {
	storerPid, err := actorContext.SpawnNamed(actor.PropsFromFunc(n.stateStorer.Receive), n.name+"-state-storer")
	if err != nil {
		panic(err)
	}
	n.stateStorerPid = storerPid
}

func (n *Node) setupValidation(actorContext actor.Context) error {
	ctx := n.startCtx

	validator, err := NewTransactionValidator(ctx, n.logger, n.notaryGroup, actorContext.Self())
	if err != nil {
		return fmt.Errorf("error setting up: %v", err)
	}
	n.validator = validator

	err = n.pubsub.RegisterTopicValidator(n.notaryGroup.Config().TransactionTopic, validator.validate)
	if err != nil {
		return fmt.Errorf("error registering topic validator: %v", err)
	}

	return nil
}

func (n *Node) subscribeToTransactions() error {
	sub, err := n.pubsub.Subscribe(n.notaryGroup.Config().TransactionTopic)
	if err != nil {
		return fmt.Errorf("error subscribing %v", err)
	}

	// don't do anything with these messages because we actually get them
	// from the topic validator which sends them in fully decoded to the
	// node main actor
	go func() {
		ctx := n.startCtx

		for {
			_, err := sub.Next(ctx)
			if err != nil {
				n.logger.Warningf("error getting sub message: %v", err)
				return
			}
		}
	}()

	return nil
}

func (n *Node) setupTickers() {
	go func() {
		ctx := n.startCtx
		// every once in a while check to see if the mempool has entries and if the
		// snowballer has started and start the snowballer if it isn't
		// this is handled by the snowball actor spun up in the node's root actor (see: Receive function)
		checkSnowballTicker := time.NewTicker(200 * time.Millisecond)
		republishTicker := time.NewTicker(500 * time.Millisecond)
		ctxDone := ctx.Done()
		for {
			select {
			case <-ctxDone:
				checkSnowballTicker.Stop()
				republishTicker.Stop()
				return
			case <-checkSnowballTicker.C:
				n.maybeStartSnowball(ctx)
			case <-republishTicker.C:
				n.maybeRepublish(ctx)
			}
		}
	}()

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
		if err := n.setupValidation(actorContext); err != nil {
			panic(err)
		}
		n.setupSyncer(actorContext)
		n.setupSnowball(actorContext)
		n.setupStateStorer(actorContext)
		n.setupTickers()
		if err := n.subscribeToTransactions(); err != nil {
			panic(err)
		}
	case *AddBlockWrapper:
		n.handleAddBlockRequest(actorContext, msg)
	case *snowballerDone:
		n.handleSnowballerDone(actorContext, msg)
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

func (n *Node) publishCompletedRound(ctx context.Context, round *round) error {
	currentStateCid, err := n.hamtStore.Put(ctx, round.state.hamt)
	if err != nil {
		return fmt.Errorf("error getting current state cid: %v", err)
	}
	n.logger.Debugf("publishCompletedRound height %d currentState: %s", round.height, currentStateCid.String())

	if !round.snowball.Decided() {
		return fmt.Errorf("can't publish an undecided round")
	}

	err = n.dagStore.Add(ctx, round.snowball.Preferred().Checkpoint.Wrapped())
	if err != nil {
		return fmt.Errorf("error adding to dag store: %w", err)
	}

	preferredCheckpointCid := round.snowball.Preferred().Checkpoint.CID()

	completedRound := &gossip.Round{
		Height:        round.height,
		StateCid:      currentStateCid.Bytes(),
		CheckpointCid: preferredCheckpointCid.Bytes(),
	}

	wrappedCompletedRound := types.WrapRound(completedRound)

	err = n.dagStore.Add(ctx, wrappedCompletedRound.Wrapped())
	if err != nil {
		return fmt.Errorf("error adding to dag store: %w", err)
	}

	err = n.storeCompletedRound(wrappedCompletedRound)
	if err != nil {
		return fmt.Errorf("could not store completed round CID: %w", err)
	}

	conf, err := n.confirmCompletedRound(ctx, wrappedCompletedRound)
	if err != nil {
		return fmt.Errorf("error confirming completed round: %v", err)
	}

	n.logger.Debugf("publishing round confirmed to: %s", n.notaryGroup.ID)

	defer func() { round.published = true }()

	return n.pubsub.Publish(n.notaryGroup.ID, conf.Data())
}

func (n *Node) storeCompletedRound(round *types.RoundWrapper) error {
	heightBytes := make([]byte, binary.MaxVarintLen64)
	binary.PutUvarint(heightBytes, round.Height())
	err := n.dataStore.Put(datastore.NewKey(lastCompletedHeightKey), heightBytes)
	if err != nil {
		return fmt.Errorf("error storing completed round: %w", err)
	}

	key := roundKey(round.Height())
	err = n.dataStore.Put(key, round.CID().Bytes())
	if err != nil {
		return fmt.Errorf("error storing round: %w", err)
	}

	return nil
}

func (n *Node) handleSnowballerDone(actorContext actor.Context, msg *snowballerDone) {
	sp := opentracing.StartSpan("gossip4.round.done")
	defer sp.Finish()

	if msg.err != nil {
		n.logger.Errorf("snowballer crashed: %v", msg.err)
		// TODO: How should we recover from this?
		return
	}

	preferred := n.snowballer.snowball.Preferred()

	completedRound := n.rounds.Current()
	completedRound.sp.SetTag("txCount", len(preferred.Checkpoint.AddBlockRequests()))
	completedRound.sp.SetTag("totalCount", completedRound.snowball.totalCount)
	completedRound.sp.Finish()
	n.logger.Infof("round %d decided with err: %v: %s (len: %d)", completedRound.height, msg.err, preferred.ID(), len(preferred.Checkpoint.AddBlockRequests()))
	n.logger.Debugf("round %d transactions %v", completedRound.height, preferred.Checkpoint.AddBlockRequests)
	// take all the transactions from the decided round, remove them from the mempool and apply them to the state
	// increase the currentRound and create a new Round in the roundHolder
	// state updating should be more robust here to make sure transactions don't stomp on each other and can probably happen in the background
	// probably need to recheck what's left in the mempool too for now we're ignoring all that as a PoC

	rootNodeSp := opentracing.StartSpan("gossip4.getRoot", opentracing.ChildOf(sp.Context()))

	var state *globalState
	if completedRound.height == 0 {
		state = newGlobalState(n.hamtStore)
	} else {
		previousRound, _ := n.rounds.Get(completedRound.height - 1)
		n.logger.Debugf("previous state for %d: %v", completedRound.height-1, previousRound.state)
		state = previousRound.state.Copy()
	}
	n.logger.Debugf("current round: %d, node: %v", completedRound, state.hamt)
	rootNodeSp.Finish()

	processSp := opentracing.StartSpan("gossip4.processTxs", opentracing.ChildOf(sp.Context()))
	abrCIDBytes := preferred.Checkpoint.AddBlockRequests()
	abrws := make([]*AddBlockWrapper, len(abrCIDBytes))
	abrCIDs := make([]cid.Cid, len(abrCIDBytes))
	for i, cidBytes := range abrCIDBytes {
		abrCid, err := cid.Cast(cidBytes)
		if err != nil {
			panic(fmt.Errorf("error casting add block request cid: %v", err))
		}
		abrCIDs[i] = abrCid
		abrWrapper := n.mempool.Get(abrCid)
		abr := abrWrapper.AddBlockRequest
		abrws[i] = abrWrapper
		nextKey := inFlightID(abr.ObjectId, abr.Height+1)
		// if the next height Tx is here we can also queue that up
		next, ok := n.inflight.Get(nextKey)
		if ok {
			n.logger.Debugf("found %s height: %d in inflight", abr.ObjectId, abr.Height+1)
			nextAbrWrapper := next.(*AddBlockWrapper)
			if bytes.Equal(nextAbrWrapper.AddBlockRequest.PreviousTip, abrWrapper.AddBlockRequest.NewTip) {
				go func() {
					if err := n.storeAbr(msg.ctx, nextAbrWrapper); err != nil {
						n.logger.Warningf("error storing abr: %v", err)
					}
				}()

			}
			n.inflight.Remove(nextKey)
		}
	}
	state.addInflights(abrws...)
	n.mempool.BulkDelete(abrCIDs...)

	processSp.Finish()

	n.logger.Debugf("setting round at %d to rootNode: %v", completedRound.height, state.hamt)
	completedRound.state = state
	go func(n *Node, completedRound *round) {
		if err := completedRound.state.backgroundProcess(context.TODO(), n, completedRound); err != nil {
			n.logger.Errorf("error processing round: %v", err)
		}
	}(n, completedRound)
	n.logger.Debugf("after setting: %v", completedRound.state.hamt)

	round := newRound(completedRound.height+1, 0, 0, min(defaultK, int(n.notaryGroup.Size())-1))
	n.rounds.SetCurrent(round)
	n.snowballer = newSnowballer(n, round.height, round.snowball)
	n.maybeStartSnowball(n.startCtx)
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
	defer s.Close()

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
	sp := abrWrapper.NewSpan("gossip4.storeAbr.Put")

	id, err := n.hamtStore.Put(ctx, abrWrapper.AddBlockRequest)
	if err != nil {
		return fmt.Errorf("error putting abr: %w", err)
	}
	sp.Finish()

	abrWrapper.SetTag("id", id.String())

	abrWrapper.cid = id

	n.logger.Debugf("storing in mempool %s", id.String())
	n.mempool.Add(abrWrapper)

	return nil
}

func (n *Node) getCurrent(ctx context.Context, objectID string) (*services.AddBlockRequest, error) {
	currentRound := n.rounds.Current()
	if currentRound.height == 0 {
		return nil, nil // this is the genesis round: we, by definition, have no state
	}

	lockedRound, ok := n.rounds.Get(currentRound.height - 1)
	if !ok {
		return nil, fmt.Errorf("we don't have the previous round")
	}

	return lockedRound.state.Find(ctx, objectID)
}

func (n *Node) PID() *actor.PID {
	return n.pid
}
