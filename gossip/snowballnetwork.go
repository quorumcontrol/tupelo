package gossip

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/opentracing/opentracing-go"

	lru "github.com/hashicorp/golang-lru"
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-msgio"
	"github.com/multiformats/go-multihash"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/gossip"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"

	"github.com/quorumcontrol/tupelo-go-sdk/gossip/types"
)

const paralellCount = 2

var ErrNilRound = errors.New("nil round")

type snowballer struct {
	sync.RWMutex

	node     *Node
	snowball *Snowball
	height   uint64
	host     p2p.Node
	group    *types.NotaryGroup
	logger   logging.EventLogger
	cache    *lru.Cache

	parentCtx     context.Context
	parentCtxStop context.CancelFunc

	respChan chan *snowballVoteResp

	started bool
}

type snowballVoteResp struct {
	signerID          string
	wrappedCheckpoint *types.CheckpointWrapper
}

func newSnowballer(n *Node, height uint64, snowball *Snowball) *snowballer {
	cache, err := lru.New(50)
	if err != nil {
		panic(err)
	}
	return &snowballer{
		node:     n,
		snowball: snowball,
		host:     n.p2pNode,
		group:    n.notaryGroup,
		logger:   n.logger,
		cache:    cache,
		height:   height,
		respChan: make(chan *snowballVoteResp, snowball.k*paralellCount),
	}
}

func (snb *snowballer) Started() bool {
	snb.RLock()
	defer snb.RUnlock()
	return snb.started
}

// Start is idempotent and thread safe
func (snb *snowballer) Start(ctx context.Context) {
	snb.Lock()
	defer snb.Unlock()

	if snb.started {
		return
	}
	snb.started = true

	preferred := snb.node.mempool.Preferred()
	snb.logger.Debugf("starting snowballer (a: %f, k: %d, b: %d) (height: %d) and preferring %v", snb.snowball.alpha, snb.snowball.k, snb.snowball.beta, snb.height, preferred)
	if len(preferred) > 0 {
		preferredBytes := make([][]byte, len(preferred))
		for i, pref := range preferred {
			preferredBytes[i] = pref.Bytes()
		}
		cp := &gossip.Checkpoint{
			Height:           snb.height,
			AddBlockRequests: preferredBytes,
		}
		snb.snowball.PreferInLock(&Vote{
			Checkpoint: types.WrapCheckpoint(cp),
		})
		snb.logger.Debugf("preferred id: %s", snb.snowball.Preferred().ID())
	}

	snowballerCtx, cancel := context.WithCancel(ctx)
	snb.parentCtx = snowballerCtx
	snb.parentCtxStop = cancel

	stopSampling := snowballerCtx.Done()

	// start populating results
	go func() {
		tokenCh := make(chan struct{}, snb.snowball.k*paralellCount)
		for i := 0; i < snb.snowball.k*paralellCount; i++ {
			tokenCh <- struct{}{}
		}

		for {
			select {
			case <-stopSampling:
				return // stop the sample
			case <-tokenCh:
				go snb.getOneRandomVote(snb.parentCtx, tokenCh)
			}

		}
	}()

	go func() {
		done := make(chan error, 1)
		snb.run(ctx, done)

		ctxDone := snowballerCtx.Done()

		select {
		case <-ctxDone:
			return
		case err := <-done:
			snb.parentCtxStop()
			snb.node.rootContext.Send(snb.node.pid, &snowballerDone{err: err, ctx: ctx})
		}
	}()

}

func (snb *snowballer) run(startCtx context.Context, done chan error) {
	sp := opentracing.StartSpan("gossip4.snowballer.start")
	defer sp.Finish()
	ctx := opentracing.ContextWithSpan(startCtx, sp)
	ctxDone := ctx.Done()

	for !snb.snowball.Decided() {
		// check to make sure ctx isn't done, if it is
		// we should stop looping
		select {
		case <-ctxDone:
			break
		default:
			//nothing to do here
		}

		snb.doTick(ctx)
	}
	// if the snowballer just found that the round is nil
	// tell the node that.
	if snb.snowball.Preferred() == nil {
		done <- ErrNilRound
		return
	}

	done <- nil
}

func (snb *snowballer) doTick(startCtx context.Context) {
	sp := opentracing.StartSpan("gossip4.snowballer.tick", opentracing.FollowsFrom(opentracing.SpanFromContext(startCtx).Context()))
	defer sp.Finish()
	ctx := opentracing.ContextWithSpan(startCtx, sp)

	sp.LogKV("height", snb.height)

	sampleSize := snb.snowball.k

	responses := make([]*snowballVoteResp, sampleSize)
	for i := 0; i < sampleSize; i++ {
		responses[i] = <-snb.respChan // this channel is being populated in another go routine
	}
	votes := make([]*Vote, sampleSize)
	for i, resp := range responses {
		if resp == nil {
			v := &Vote{}
			v.Nil()
			votes[i] = v
			continue
		}
		wrappedCheckpoint := resp.wrappedCheckpoint
		snb.logger.Debugf("%s checkpoint: %v", resp.signerID, wrappedCheckpoint.Value())
		vote := &Vote{
			Checkpoint: wrappedCheckpoint,
		}
		abrs := wrappedCheckpoint.AddBlockRequests()
		if len(abrs) == 0 ||
			!(snb.mempoolHasAllABRs(abrs)) ||
			snb.hasConflictingABRs(abrs) {
			// nil out any votes that have ABRs we havne't heard of
			// or if they present conflicting ABRs in the same Checkpoint
			snb.logger.Debugf("%s nilling vote", resp.signerID)
			vote.Nil()
		}
		votes[i] = vote
	}

	votes = calculateTallies(votes)
	// snb.logger.Debugf("votes: %s", spew.Sdump(votes))

	snb.snowball.Tick(ctx, votes)
	// snb.logger.Debugf("counts: %v, beta: %d", snb.snowball.counts, snb.snowball.count)
}

func (snb *snowballer) getOneRandomVote(parentCtx context.Context, tokenCh chan struct{}) {
	sp, ctx := opentracing.StartSpanFromContext(parentCtx, "gossip4.snowballer.getOneRandomVote")
	defer sp.Finish()

	var resp *snowballVoteResp // set up here so that we can add a nil response to the resp channel on error

	var signer *types.Signer
	var signerPeer string
	defer func() {
		select {
		case snb.respChan <- resp: // this will block when we don't need any more responses
			// do nothing
		case <-ctx.Done():
			return // no need to return the token
		}
		select {
		case tokenCh <- struct{}{}: // and then this will let the sampler continue to get more responses
			return
		case <-ctx.Done():
			return
		}
	}()

	// pick one non-self signer
	for signer == nil || signerPeer == snb.host.Identity() {
		signer = snb.group.GetRandomSigner()
		peerID, _ := p2p.PeerIDFromPublicKey(signer.DstKey)
		signerPeer = peerID.Pretty()
	}
	snb.logger.Debugf("snowballing with %s", signerPeer)
	sp.LogKV("signerPeer", signerPeer)

	s, err := snb.host.NewStream(ctx, signer.DstKey, gossipProtocol)
	if err != nil {
		sp.LogKV("error", err.Error())
		snb.logger.Warningf("error creating stream to %s: %v", signer.ID, err)
		if s != nil {
			s.Close()
		}
		return
	}
	defer s.Close()

	sw := &safewrap.SafeWrap{}
	wrapped := sw.WrapObject(snb.height)

	if err := s.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		sp.LogKV("error", err.Error())
		snb.logger.Errorf("error setting deadline: %v", err) // TODO: do we need to do anything about this error?
		return
	}

	writer := msgio.NewVarintWriter(s)
	err = writer.WriteMsg(wrapped.RawData())
	if err != nil {
		sp.LogKV("error", err.Error())
		snb.logger.Warningf("error writing to stream to %s: %v", signer.ID, err)
		return
	}

	reader := msgio.NewVarintReader(s)
	bits, err := reader.ReadMsg()
	if err != nil {
		sp.LogKV("error", err.Error())
		snb.logger.Warningf("error reading from stream to %s: %v", signer.ID, err)
		return
	}

	id, err := cidFromBits(bits)
	if err != nil {
		sp.LogKV("error", err.Error())
		snb.logger.Warningf("error creating bits to %s: %v", signer.ID, err)
		return
	}

	var wrappedCheckpoint *types.CheckpointWrapper

	blkInter, ok := snb.cache.Get(id)
	if ok {
		sp.LogKV("cacheHit", true)
		wrappedCheckpoint = blkInter.(*types.CheckpointWrapper)
	} else {
		sp.LogKV("cacheHit", false)
		blk := &gossip.Checkpoint{}
		err = cbornode.DecodeInto(bits, blk)
		if err != nil {
			sp.LogKV("error", err.Error())
			snb.logger.Warningf("error decoding from stream to %s: %v", signer.ID, err)
			return
		}
		wrappedCheckpoint = types.WrapCheckpoint(blk)
		snb.cache.Add(id, wrappedCheckpoint)
	}

	resp = &snowballVoteResp{
		signerID:          signerPeer,
		wrappedCheckpoint: wrappedCheckpoint,
	}
}

func cidFromBits(bits []byte) (cid.Cid, error) {
	hash, err := multihash.Sum(bits, multihash.SHA2_256, -1)
	if err != nil {
		return cid.Undef, err
	}
	return cid.NewCidV1(cid.DagCBOR, hash), nil
}

func (snb *snowballer) mempoolHasAllABRs(abrCIDs [][]byte) bool {
	hasAll := true
	for _, cidBytes := range abrCIDs {
		abrCID, err := cid.Cast(cidBytes)
		if err != nil {
			panic(fmt.Errorf("error casting add block request cid: %v", err))
		}

		ok := snb.node.mempool.Contains(abrCID)
		if !ok {
			snb.logger.Debugf("missing tx: %s", abrCID.String())
			snb.node.rootContext.Send(snb.node.syncerPid, abrCID)
			// the reason to not just return false here is that we want
			// to continue to loop over all the Txs in order to sync the ones we don't have
			hasAll = false
		}
	}
	snb.logger.Debugf("hasAllABRs: %t", hasAll)
	return hasAll
}

// this checks to make sure the block coming in doesn't have any conflicting transactions
func (snb *snowballer) hasConflictingABRs(abrCIDs [][]byte) bool {
	csIds := make(map[mempoolConflictSetID]struct{})
	for _, cidBytes := range abrCIDs {
		abrCID, err := cid.Cast(cidBytes)
		if err != nil {
			panic(fmt.Errorf("error casting add block request cid: %v", err))
		}

		wrapper := snb.node.mempool.Get(abrCID)
		if wrapper == nil {
			snb.logger.Errorf("had a null transaction ( %s ) from a block in the mempool, this shouldn't happen", abrCID.String())
			return true
		}
		conflictSetID := toConflictSetID(wrapper.AddBlockRequest)
		_, ok := csIds[conflictSetID]
		if ok {
			snb.logger.Debugf("hasConflicting: true")
			return true
		}
		csIds[conflictSetID] = struct{}{}
	}
	snb.logger.Debugf("hasConflicting: false")
	return false
}

type snowballerDone struct {
	err error
	ctx context.Context
}
