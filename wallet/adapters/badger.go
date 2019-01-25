package adapters

import (
	"fmt"

	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/storage"
)

type BadgerStorageAdapter struct {
	store *nodestore.StorageBasedStore
}

func NewBadgerStorage(config map[string]interface{}) (*BadgerStorageAdapter, error) {
	path, ok := config["path"]

	if !ok {
		return nil, fmt.Errorf("Badger requires path in StorageConfig")
	}

	db, err := storage.NewBadgerStorage(path.(string))

	if err != nil {
		return nil, fmt.Errorf("Error initializing badger storage: %v", err)
	}

	return &BadgerStorageAdapter{
		store: nodestore.NewStorageBasedStore(db),
	}, nil
}

func (a *BadgerStorageAdapter) Store() nodestore.NodeStore {
	return a.store
}

func (a *BadgerStorageAdapter) Close() error {
	a.store.Close()
	return nil
}
