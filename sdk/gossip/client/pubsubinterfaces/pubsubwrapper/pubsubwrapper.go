package pubsubwrapper

import (
	"context"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/quorumcontrol/tupelo/sdk/gossip/client/pubsubinterfaces"
)

// compile-time assertion that this wrapper implements the required interfaces
var _ pubsubinterfaces.Pubsubber = (*libp2pWrapper)(nil)
var _ pubsubinterfaces.Subscription = (*libp2pSubscriptionWrapper)(nil)

// WrapLibp2p wraps a libp2p pubsub instance into the needed client interfaces
func WrapLibp2p(libp2ppubsub *pubsub.PubSub) pubsubinterfaces.Pubsubber {
	return &libp2pWrapper{
		PubSub: libp2ppubsub,
	}
}

type libp2pWrapper struct {
	*pubsub.PubSub
}

func (lpw *libp2pWrapper) Subscribe(topic string) (pubsubinterfaces.Subscription, error) {
	sub, err := lpw.PubSub.Subscribe(topic)
	return &libp2pSubscriptionWrapper{sub}, err
}

func (lpw *libp2pWrapper) Publish(topic string, data []byte) error {
	return lpw.PubSub.Publish(topic, data)
}

type libp2pSubscriptionWrapper struct {
	*pubsub.Subscription
}

func (lpsw *libp2pSubscriptionWrapper) Next(ctx context.Context) (pubsubinterfaces.Message, error) {
	return lpsw.Subscription.Next(ctx)
}
