package gossip

import (
	"context"
	"fmt"
	"sync"
	"time"

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

type snowballer struct {
	sync.RWMutex

	node     *Node
	snowball *Snowball
	height   uint64
	host     p2p.Node
	group    *types.NotaryGroup
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
		respChan := make(chan gossip.Checkpoint, snb.snowball.k)
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
				s, err := snb.host.NewStream(ctx, signer.DstKey, gossipProtocol)
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

				if err := s.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
					snb.logger.Errorf("error setting deadline: %v", err) // TODO: do we need to do anything about this error?
				}

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

				var checkpoint *gossip.Checkpoint

				blkInter, ok := snb.cache.Get(id)
				if ok {
					checkpoint = blkInter.(*gossip.Checkpoint)
				} else {
					blk := &gossip.Checkpoint{}
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
				Checkpoint: types.WrapCheckpoint(&checkpoint),
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
			hasAll = false
		}
	}
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
