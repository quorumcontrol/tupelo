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

type transactionGetter struct {
	nodeActor *actor.PID
	store     *hamt.CborIpldStore
	logger    logging.EventLogger
	validator *TransactionValidator
	cache     *lru.Cache
}

func (tg *transactionGetter) Receive(actorContext actor.Context) {
	switch msg := actorContext.Message().(type) {
	case *actor.Started:
		cache, err := lru.New(500)
		if err != nil {
			tg.logger.Errorf("error creating cache: %v", err)
			panic("error creating cache")
		}
		tg.cache = cache
	case cid.Cid:
		if tg.cache.Contains(msg.String()) {
			return
		}

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		abr := &services.AddBlockRequest{}
		err := tg.store.Get(ctx, msg, abr)
		if err != nil {
			tg.logger.Warningf("error fetching %s", msg.String())
			return
		}

		tg.cache.Add(msg.String(), struct{}{})

		wrapper := &AddBlockWrapper{
			AddBlockRequest: abr,
		}
		wrapper.StartTrace("gossip4.syncer")

		newTip, valid, newNodes, err := tg.validator.ValidateAbr(wrapper)
		if valid {
			wrapper.SetTag("valid", true)
			wrapper.AddBlockRequest.NewTip = newTip.Bytes()
			wrapper.NewNodes = newNodes
			tg.logger.Debugf("sending %s to the node", msg.String())
			actorContext.Send(tg.nodeActor, wrapper)
			return
		}
		wrapper.SetTag("valid", false)
		wrapper.StopTrace()
		tg.logger.Warningf("received invalid transaction: %s", msg.String())

	}
}
