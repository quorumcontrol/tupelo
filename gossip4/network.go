package gossip4

import (
	"context"
	"sync"
	"time"

	"github.com/davecgh/go-spew/spew"
	cbornode "github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-msgio"

	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"

	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
)

type snowballer struct {
	sync.RWMutex

	snowball *Snowball
	height   uint64
	host     p2p.Node
	group    *types.NotaryGroup
	logger   logging.EventLogger
	started  bool
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
				var block Block
				err = cbornode.DecodeInto(bits, &block)
				if err != nil {
					snb.logger.Warningf("error decoding from stream to %s: %v", signer.ID, err)
					s.Close()
					wg.Done()
					return
				}
				respChan <- block
				wg.Done()
			}()
		}
		wg.Wait()
		close(respChan)
		votes := make([]*Vote, snb.snowball.k)
		i := 0
		for block := range respChan {
			snb.logger.Debugf("received block: %v", block)
			//TODO: here we should throw away blocks with Txs we don't know about.
			votes[i] = &Vote{
				Block: &block,
			}
			if len(block.Transactions) == 0 {
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
		snb.logger.Debugf("votes: %s", spew.Sdump(votes))

		snb.snowball.Tick(votes)
		snb.logger.Debugf("counts: %v, beta: %d", snb.snowball.counts, snb.snowball.count)
	}

	done <- nil
}
