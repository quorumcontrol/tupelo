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

	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"

	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
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
		respChan := make(chan Block, snb.snowball.k)
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

				var block *Block

				blkInter, ok := snb.cache.Get(id)
				if ok {
					block = blkInter.(*Block)
				} else {
					blk := &Block{}
					err = cbornode.DecodeInto(bits, blk)
					if err != nil {
						snb.logger.Warningf("error decoding from stream to %s: %v", signer.ID, err)
						s.Close()
						wg.Done()
						return
					}
					block = blk
					snb.cache.Add(id, block)
				}

				respChan <- *block
				wg.Done()
			}()
		}
		wg.Wait()
		close(respChan)
		votes := make([]*Vote, snb.snowball.k)
		i := 0
		for block := range respChan {
			// snb.logger.Debugf("received block: %v", block)
			votes[i] = &Vote{
				Block: &block,
			}
			if len(block.Transactions) == 0 || !snb.mempoolHasAllTransactions(block.Transactions) {
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

func (snb *snowballer) mempoolHasAllTransactions(transactions []cid.Cid) bool {
	hasAll := true
	for _, txCID := range transactions {
		ok := snb.node.mempool.Contains(txCID)
		if !ok {
			snb.logger.Debugf("missing tx: %s", txCID.String())
			snb.node.rootContext.Send(snb.node.syncerPid, txCID)
			hasAll = false
		}
	}
	return hasAll
}

type snowballerDone struct {
	err error
	ctx context.Context
}
