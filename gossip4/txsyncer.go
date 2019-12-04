package gossip4

import (
	"context"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	logging "github.com/ipfs/go-log"
	"github.com/quorumcontrol/messages/v2/build/go/services"
)

type transactionGetter struct {
	nodeActor *actor.PID
	store     *hamt.CborIpldStore
	logger    logging.EventLogger
}

func (tg *transactionGetter) Receive(actorContext actor.Context) {
	switch msg := actorContext.Message().(type) {
	case cid.Cid:
		ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer cancel()
		abr := &services.AddBlockRequest{}
		err := tg.store.Get(ctx, msg, abr)
		if err != nil {
			tg.logger.Warningf("error fetching %s", msg.String())
		}
		actorContext.Send(tg.nodeActor, abr)
	}
}
