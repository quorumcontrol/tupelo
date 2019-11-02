package gossip4

import (
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	logging "github.com/ipfs/go-log"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/quorumcontrol/chaintree/safewrap"
)

type rebroadcaster struct {
	latest *Checkpoint
	logger logging.EventLogger
	pubsub *pubsub.PubSub
}

func (r *rebroadcaster) Receive(actorContext actor.Context) {
	switch msg := actorContext.Message().(type) {
	case *Checkpoint:
		actorContext.SetReceiveTimeout(10 * time.Millisecond)
		if r.latest == nil ||
			r.latest.SignerCount() < msg.SignerCount() ||
			r.latest.Height < msg.Height {
			r.logger.Debugf("replacing latest with %d %v", msg.Height, msg.Signature.Signers)
			r.latest = msg
			return
		}
	case *actor.ReceiveTimeout:
		sw := &safewrap.SafeWrap{}
		nd := sw.WrapObject(r.latest)
		if sw.Err != nil {
			r.logger.Warningf("error wrapping latest: %v", sw.Err)
			return
		}
		r.logger.Debugf("rebroadcaster sending %d %v", r.latest.Height, r.latest.Signature.Signers)
		r.pubsub.Publish(commitTopic, nd.RawData())
	}
}
