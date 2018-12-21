package storage

import (
	"github.com/hashicorp/go-immutable-radix"
	"github.com/quorumcontrol/storage"
)

// MemStorage implements the Storage interface using an in-memory
// only store with NO locks (because write-access is considered single threaded)
type MemStorage struct {
	tree *iradix.Tree
}

var _ storage.Storage = (*MemStorage)(nil)

// NewMemStorage returns a memory-only (MemStorage) implementation of the Storage interface
func NewMemStorage() *MemStorage {
	return &MemStorage{
		tree: iradix.New(),
	}
}

// Close implements the Storage interface
// It is a NoOp on memstorage.
func (ms *MemStorage) Close() {
	// noop
}

// SetIfNotExists implements the Storage interface
func (ms *MemStorage) SetIfNotExists(key []byte, value []byte) (bool, error) {
	_, exists := ms.tree.Get(key)
	if !exists {
		newTree, _, _ := ms.tree.Insert(key, value)
		ms.tree = newTree
		return true, nil
	}

	return false, nil
}

// Set implements the Storage interface
func (ms *MemStorage) Set(key, value []byte) error {
	newTree, _, _ := ms.tree.Insert(key, value)
	ms.tree = newTree
	return nil
}

// Delete implements the Storage interface
func (ms *MemStorage) Delete(key []byte) error {
	newTree, _, didDelete := ms.tree.Delete(key)
	if !didDelete {
		return nil
	}
	ms.tree = newTree
	return nil
}

// Get implements the Storage interface
func (ms *MemStorage) Get(key []byte) ([]byte, error) {
	val, ok := ms.tree.Get(key)
	if ok {
		return val.([]byte), nil
	}
	return nil, nil
}

// GetAll implements the Storage interface
func (ms *MemStorage) GetAll(keys [][]byte) ([]storage.KeyValuePair, error) {
	tree := ms.tree

	pairs := make([]storage.KeyValuePair, len(keys))
	for i, key := range keys {
		var value []byte
		maybeValue, ok := tree.Get(key)
		if ok {
			value = maybeValue.([]byte)
		}
		pairs[i] = storage.KeyValuePair{
			Key:   key,
			Value: value,
		}
	}
	return pairs, nil
}

// Exists implements the Storage interface
func (ms *MemStorage) Exists(key []byte) (bool, error) {
	_, ok := ms.tree.Get(key)
	return ok, nil
}

// GetPairsByPrefix implements the Storage interface
func (ms *MemStorage) GetPairsByPrefix(prefix []byte) ([]storage.KeyValuePair, error) {
	var pairs []storage.KeyValuePair
	ms.prefixIterator(prefix, func(key, value []byte) error {
		pairs = append(pairs, storage.KeyValuePair{
			Key:   key,
			Value: value,
		})
		return nil
	})
	return pairs, nil
}

// GetKeysByPrefix implements the Storage interface
func (ms *MemStorage) GetKeysByPrefix(prefix []byte) ([][]byte, error) {
	var keys [][]byte
	ms.prefixIterator(prefix, func(key, value []byte) error {
		keys = append(keys, key)
		return nil
	})
	return keys, nil
}

// ForEach implements the Storage interface
func (ms *MemStorage) ForEach(prefix []byte, iterator func(k, v []byte) error) error {
	return ms.prefixIterator(prefix, iterator)
}

// ForEachKey implements the Storage interface
func (ms *MemStorage) ForEachKey(prefix []byte, iterator func(k []byte) error) error {
	return ms.prefixIterator(prefix, func(key, value []byte) error {
		return iterator(key)
	})
}

func (ms *MemStorage) prefixIterator(prefix []byte, iterator func(key, value []byte) error) error {
	var err error
	ms.tree.Root().WalkPrefix(prefix, func(key []byte, value interface{}) bool {
		err = iterator(key, value.([]byte))
		if err != nil {
			return true
		}
		return false
	})

	return err
}

// Fork returns a copy of the memstorage
// which can move forward with its own history
func (ms *MemStorage) Fork() *MemStorage {
	newStore := NewMemStorage()
	newStore.tree = ms.tree
	return newStore
}
