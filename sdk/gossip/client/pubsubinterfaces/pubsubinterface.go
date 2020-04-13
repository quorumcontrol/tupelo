package pubsubinterfaces

import (
	"context"
)

// These various interfaces are used instead of pubsub.Pubsub because that particular package
// brings in dependencies that are not supported in WASM. Using an interface here lets the WASM
// packages avoid bringing in the pubsub packages, and still use this client.

// PubsubMessage is the equivalent of a *pubsub.Message
type Message interface {
	GetData() []byte
}

// PubsubSubscription is the equivalent of a *pubsub.Subscription
type Subscription interface {
	Next(context.Context) (Message, error)
	Cancel()
}

// Pubsubber is ther interface needed for a client to a *pubsub.Pubsub
type Pubsubber interface {
	Publish(topic string, data []byte) error
	Subscribe(topic string) (Subscription, error)
}
