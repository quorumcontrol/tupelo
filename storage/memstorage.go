package storage

import (
	"sync"

	"github.com/ethereum/go-ethereum/log"
)

type MemBucket struct {
	Keys map[string][]byte
	lock *sync.RWMutex
}

type MemStorage struct {
	Buckets map[string]*MemBucket
	lock    *sync.RWMutex
}

var _ Storage = (*MemStorage)(nil)

func NewMemStorage() *MemStorage {
	return &MemStorage{
		Buckets: make(map[string]*MemBucket),
		lock:    &sync.RWMutex{},
	}
}

func (ms *MemStorage) Close() {
	//noop
}

func (ms *MemStorage) CreateBucketIfNotExists(bucketName []byte) error {
	ms.lock.Lock()
	defer ms.lock.Unlock()
	_, ok := ms.Buckets[string(bucketName)]
	if !ok {
		ms.Buckets[string(bucketName)] = &MemBucket{
			Keys: make(map[string][]byte),
			lock: &sync.RWMutex{},
		}
	}
	return nil
}

func (ms *MemStorage) Set(bucketName []byte, key []byte, value []byte) error {
	ms.lock.RLock()
	defer ms.lock.RUnlock()

	ms.Buckets[string(bucketName)].lock.Lock()
	defer ms.Buckets[string(bucketName)].lock.Unlock()

	ms.Buckets[string(bucketName)].Keys[string(key)] = value

	return nil
}

func (ms *MemStorage) Delete(bucketName []byte, key []byte) error {
	ms.lock.RLock()
	defer ms.lock.RUnlock()

	ms.Buckets[string(bucketName)].lock.Lock()
	defer ms.Buckets[string(bucketName)].lock.Unlock()

	_, ok := ms.Buckets[string(bucketName)]
	if ok {
		delete(ms.Buckets[string(bucketName)].Keys, string(key))
	}
	return nil
}

func (ms *MemStorage) Get(bucketName []byte, key []byte) ([]byte, error) {
	ms.lock.RLock()
	defer ms.lock.RUnlock()

	ms.Buckets[string(bucketName)].lock.RLock()
	defer ms.Buckets[string(bucketName)].lock.RUnlock()

	val, ok := ms.Buckets[string(bucketName)].Keys[string(key)]
	if ok {
		return val, nil
	}
	return nil, nil
}

func (ms *MemStorage) GetKeys(bucketName []byte) ([][]byte, error) {
	ms.lock.RLock()
	defer ms.lock.RUnlock()

	ms.Buckets[string(bucketName)].lock.RLock()
	defer ms.Buckets[string(bucketName)].lock.RUnlock()

	keys := make([][]byte, len(ms.Buckets[string(bucketName)].Keys))
	i := 0
	for k := range ms.Buckets[string(bucketName)].Keys {
		keys[i] = []byte(k)
		i++
	}
	return keys, nil
}

func (ms *MemStorage) ForEach(bucketName []byte, iterator func(k, v []byte) error) error {
	var err error
	ms.lock.RLock()
	defer ms.lock.RUnlock()

	bucket, ok := ms.Buckets[string(bucketName)]
	if !ok {
		log.Crit("unknown bucket", "bucket", string(bucketName))
	}

	bucket.lock.RLock()
	defer bucket.lock.RUnlock()

	for k, v := range bucket.Keys {
		err = iterator([]byte(k), v)
		if err != nil {
			break
		}
	}

	return err
}
