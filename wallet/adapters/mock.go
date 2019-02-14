package adapters

import (
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/storage"
)

const MockStorageAdapterName = "mock"

type MockStorageAdapter struct {
	store *nodestore.StorageBasedStore
}

func NewMockStorage(config map[string]interface{}) (*MockStorageAdapter, error) {
	db := storage.NewMemStorage()

	return &MockStorageAdapter{
		store: nodestore.NewStorageBasedStore(db),
	}, nil
}

func (a *MockStorageAdapter) Store() nodestore.NodeStore {
	return a.store
}

func (a *MockStorageAdapter) Close() error {
	a.store.Close()
	return nil
}
