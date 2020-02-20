package gossip

import (
	"context"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	lru "github.com/hashicorp/golang-lru"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	logging "github.com/ipfs/go-log"
	"github.com/quorumcontrol/messages/v2/build/go/services"
)

const defaultTransactionSyncDelayMs = 3000

type transactionGetter struct {
	nodeActor *actor.PID
	store     *hamt.CborIpldStore
	logger    logging.EventLogger
	validator *TransactionValidator
	queue     *lru.Cache
	cache     *lru.Cache
	delay     int64
}

func (tg *transactionGetter) Receive(actorContext actor.Context) {
	switch msg := actorContext.Message().(type) {
	case *actor.Started:
		cache, err := lru.New(1000)
		if err != nil {
			tg.logger.Errorf("error creating cache: %v", err)
			panic("error creating cache")
		}
		tg.cache = cache

		queue, err := lru.New(500)
		if err != nil {
			tg.logger.Errorf("error creating queue: %v", err)
			panic("error creating queue")
		}
		tg.queue = queue

		if tg.delay == 0 {
			tg.delay = defaultTransactionSyncDelayMs
		}

		tg.logger.Warningf("TXDELAY - starting a txsyncer with %d", tg.delay)
	case cid.Cid:
		cidStr := msg.String()
		tg.logger.Warningf("TXDELAY - looking for %s", cidStr)

		// recently fetched
		if tg.cache.Contains(cidStr) {
			tg.logger.Warningf("TXDELAY - recently fetched %s", cidStr)
			return
		}

		currentTime := time.Now().UnixNano() / int64(time.Millisecond)
		firstRequestTime, ok := tg.queue.Get(cidStr)

		if !ok {
			tg.logger.Warningf("TXDELAY - starting delay for %s at %d", cidStr, currentTime)
			tg.queue.Add(cidStr, currentTime)
			return
		}

		if (firstRequestTime.(int64) + tg.delay) > currentTime {
			tg.logger.Warningf("TXDELAY - skipping fetch for %s, current %d, inception %d", cidStr, currentTime, firstRequestTime.(int64))
			// still within delay, do nothing
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		abr := &services.AddBlockRequest{}
		err := tg.store.Get(ctx, msg, abr)
		if err != nil {
			tg.logger.Warningf("error fetching %s", cidStr)
		}

		tg.cache.Add(cidStr, struct{}{})
		tg.logger.Warningf("TXDELAY - fetch completed %s, current %d, inception %d", cidStr, currentTime, firstRequestTime.(int64))

		valid := tg.validator.ValidateAbr(ctx, abr)
		if valid {
			wrapper := &AddBlockWrapper{
				AddBlockRequest: abr,
			}
			wrapper.StartTrace("gossip4.syncer")
			tg.logger.Debugf("sending %s to the node", cidStr)
			actorContext.Send(tg.nodeActor, wrapper)
			return
		}
		tg.logger.Warningf("received invalid transaction: %s", cidStr)
	default:
		tg.logger.Debugf("TXDELAY actor received unrecognized %T message: %+v", msg, msg)
	}
}
