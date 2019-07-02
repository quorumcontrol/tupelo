package adapters

import (
	"context"

	"github.com/quorumcontrol/chaintree/nodestore"
)

const MockStorageAdapterName = "mock"

type MockStorageAdapter struct {
	store nodestore.DagStore
}

func NewMockStorage(config map[string]interface{}) (*MockStorageAdapter, error) {
	return &MockStorageAdapter{
		store: nodestore.MustMemoryStore(context.TODO()),
	}, nil
}

func (a *MockStorageAdapter) Store() nodestore.DagStore {
	return a.store
}

func (a *MockStorageAdapter) Close() error {
	return nil
}
