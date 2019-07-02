package adapters

import (
	"context"
	"fmt"

	datastore "github.com/ipfs/go-datastore"

	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/tupelo/storage"
)

const BadgerStorageAdapterName = "badger"

type BadgerStorageAdapter struct {
	store  nodestore.DagStore
	db     datastore.Batching
	cancel context.CancelFunc
}

func NewBadgerStorage(config map[string]interface{}) (*BadgerStorageAdapter, error) {
	ctx, cancel := context.WithCancel(context.TODO())

	path, ok := config["path"]

	if !ok {
		cancel()
		return nil, fmt.Errorf("Badger requires path in StorageConfig")
	}

	db, err := storage.NewDefaultBadger(path.(string))

	if err != nil {
		cancel()
		return nil, fmt.Errorf("Error initializing badger storage: %v", err)
	}

	store, err := nodestore.FromDatastoreOffline(ctx, db)
	if err != nil {
		cancel()
		return nil, fmt.Errorf("error creating dagstore: %v", err)
	}

	return &BadgerStorageAdapter{
		store:  store,
		cancel: cancel,
		db:     db,
	}, nil
}

func (a *BadgerStorageAdapter) Store() nodestore.DagStore {
	return a.store
}

func (a *BadgerStorageAdapter) Close() error {
	if a.cancel != nil {
		a.cancel()
	}
	if a.db != nil {
		a.db.Close()
	}
	return nil
}
