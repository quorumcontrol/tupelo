package adapters

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/quorumcontrol/chaintree/nodestore"
)

type Adapter interface {
	Close() error
	Store() nodestore.NodeStore
}

type Config struct {
	Adapter   string
	Arguments map[string]interface{}
}

var ErrAdapterNotFound = fmt.Errorf("adapter not found")

func New(config *Config) (Adapter, error) {
	switch config.Adapter {
	case BadgerStorageAdapterName:
		return NewBadgerStorage(config.Arguments)
	case IpldStorageAdapterName:
		return NewIpldStorage(config.Arguments)
	case MockStorageAdapterName:
		return NewMockStorage(config.Arguments)
	default:
		return nil, ErrAdapterNotFound
	}
}

type AdapterSingletonFactory struct {
	cache map[string]Adapter
	lock  *sync.Mutex
}

func NewAdapterSingletonFactory() *AdapterSingletonFactory {
	return &AdapterSingletonFactory{
		cache: make(map[string]Adapter),
		lock:  &sync.Mutex{},
	}
}

func (ac *AdapterSingletonFactory) New(config *Config) (Adapter, error) {
	argBytes, err := json.Marshal(config.Arguments)

	if err != nil {
		return nil, fmt.Errorf("Could not marshal config: %v", err)
	}

	configBytes := append([]byte(config.Adapter+"|"), argBytes...)
	idHash := sha256.Sum256(configBytes)
	id := string(idHash[:])

	ac.lock.Lock()
	defer ac.lock.Unlock()

	if _, exists := ac.cache[id]; !exists {
		adapter, err := New(config)

		if err != nil {
			return nil, err
		}
		ac.cache[id] = adapter
	}

	return ac.cache[id], nil
}

func (ac *AdapterSingletonFactory) Close() error {
	ac.lock.Lock()
	defer ac.lock.Unlock()

	errs := make([]error, 0)

	for key, adapter := range ac.cache {
		err := adapter.Close()

		if err != nil {
			errs = append(errs, err)
		} else {
			delete(ac.cache, key)

		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("Error closing 1 or more adapters: %v", errs)
	}

	return nil
}
