package gossip4

import (
	"context"
	"sync"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-msgio"
	"github.com/multiformats/go-multihash"
	g4types "github.com/quorumcontrol/tupelo-go-sdk/gossip4/types"

	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"

	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	g3types "github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
)

type snowballer struct {
	sync.RWMutex

	node     *Node
	snowball *Snowball
	height   uint64
	host     p2p.Node
	group    *g3types.NotaryGroup
	logger   logging.EventLogger
	cache    *lru.Cache
	started  bool
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
	}
}

func (snb *snowballer) Started() bool {
	snb.RLock()
	defer snb.RUnlock()
	return snb.started
}

func (snb *snowballer) start(ctx context.Context, done chan error) {
	snb.Lock()
	snb.started = true
	snb.Unlock()

	for !snb.snowball.Decided() {
		respChan := make(chan g4types.Checkpoint, snb.snowball.k)
		wg := &sync.WaitGroup{}
		for i := 0; i < snb.snowball.k; i++ {
			wg.Add(1)
			go func() {
				var signer *types.Signer
				var signerPeer string
				for signer == nil || signerPeer == snb.host.Identity() {
					signer = snb.group.GetRandomSigner()
					peerID, _ := p2p.PeerIDFromPublicKey(signer.DstKey)
					signerPeer = peerID.Pretty()
				}
				s, err := snb.host.NewStream(ctx, signer.DstKey, gossip4Protocol)
				if err != nil {
					snb.logger.Warningf("error creating stream to %s: %v", signer.ID, err)
					if s != nil {
						s.Close()
					}
					wg.Done()
					return
				}
				sw := &safewrap.SafeWrap{}
				wrapped := sw.WrapObject(snb.height)

				s.SetDeadline(time.Now().Add(2 * time.Second))

				writer := msgio.NewVarintWriter(s)
				err = writer.WriteMsg(wrapped.RawData())
				if err != nil {
					snb.logger.Warningf("error writing to stream to %s: %v", signer.ID, err)
					s.Close()
					wg.Done()
					return
				}

				reader := msgio.NewVarintReader(s)
				bits, err := reader.ReadMsg()
				if err != nil {
					snb.logger.Warningf("error reading from stream to %s: %v", signer.ID, err)
					s.Close()
					wg.Done()
					return
				}

				id, err := cidFromBits(bits)
				if err != nil {
					snb.logger.Warningf("error creating bits to %s: %v", signer.ID, err)
					s.Close()
					wg.Done()
					return
				}

				var checkpoint *g4types.Checkpoint

				blkInter, ok := snb.cache.Get(id)
				if ok {
					checkpoint = blkInter.(*g4types.Checkpoint)
				} else {
					blk := &g4types.Checkpoint{}
					err = cbornode.DecodeInto(bits, blk)
					if err != nil {
						snb.logger.Warningf("error decoding from stream to %s: %v", signer.ID, err)
						s.Close()
						wg.Done()
						return
					}
					checkpoint = blk
					snb.cache.Add(id, checkpoint)
				}

				respChan <- *checkpoint
				wg.Done()
			}()
		}
		wg.Wait()
		close(respChan)
		votes := make([]*Vote, snb.snowball.k)
		i := 0
		for checkpoint := range respChan {
			// snb.logger.Debugf("received checkpoint: %v", checkpoint)
			votes[i] = &Vote{
				Checkpoint: &checkpoint,
			}
			if len(checkpoint.AddBlockRequests) == 0 ||
				!snb.mempoolHasAllABRs(checkpoint.AddBlockRequests) ||
				snb.hasConflictingABRs(checkpoint.AddBlockRequests) {
				// nil out any votes that have ABRs we havne't heard of
				// or if they present conflicting ABRs in the same Checkpoint
				votes[i].Nil()
			}

			i++
		}

		if i < len(votes) {
			for j := i; j < len(votes); j++ {
				v := &Vote{}
				v.Nil()
				votes[j] = v
			}
		}

		votes = calculateTallies(votes)
		// snb.logger.Debugf("votes: %s", spew.Sdump(votes))

		snb.snowball.Tick(votes)
		// snb.logger.Debugf("counts: %v, beta: %d", snb.snowball.counts, snb.snowball.count)
	}

	done <- nil
}

func cidFromBits(bits []byte) (cid.Cid, error) {
	hash, err := multihash.Sum(bits, multihash.SHA2_256, -1)
	if err != nil {
		return cid.Undef, err
	}
	return cid.NewCidV1(cid.DagCBOR, hash), nil
}

func (snb *snowballer) mempoolHasAllABRs(abrCIDs []cid.Cid) bool {
	hasAll := true
	for _, abrCID := range abrCIDs {
		ok := snb.node.mempool.Contains(abrCID)
		if !ok {
			snb.logger.Debugf("missing tx: %s", abrCID.String())
			snb.node.rootContext.Send(snb.node.syncerPid, abrCID)
			hasAll = false
		}
	}
	return hasAll
}

// this checks to make sure the block coming in doesn't have any conflicting transactions
func (snb *snowballer) hasConflictingABRs(abrCIDs []cid.Cid) bool {
	csIds := make(map[mempoolConflictSetID]struct{})
	for _, abrCID := range abrCIDs {
		tx := snb.node.mempool.Get(abrCID)
		if tx == nil {
			snb.logger.Errorf("had a null transaction ( %s ) from a block in the mempool, this shouldn't happen", abrCID.String())
			return true
		}
		conflictSetID := toConflictSetID(tx)
		_, ok := csIds[conflictSetID]
		if ok {
			return true
		}
		csIds[conflictSetID] = struct{}{}
	}
	return false
}

type snowballerDone struct {
	err error
	ctx context.Context
}
