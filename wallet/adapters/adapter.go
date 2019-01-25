package adapters

import (
	"fmt"

	"github.com/quorumcontrol/chaintree/nodestore"
)

type Adapter interface {
	Store() nodestore.NodeStore
	Close() error
}

type Config struct {
	Adapter   string
	Arguments map[string]interface{}
}

var ErrAdapterNotFound = fmt.Errorf("adapter not found")

func New(config *Config) (Adapter, error) {
	switch config.Adapter {
	case "badger":
		return NewBadgerStorage(config.Arguments)
	case "ipld":
		return NewIpldStorage(config.Arguments)
	default:
		return nil, ErrAdapterNotFound
	}
}
