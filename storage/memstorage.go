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
	bucket := ms.Buckets[string(bucketName)]
	bucket.lock.Lock()
	defer bucket.lock.Unlock()

	ms.Buckets[string(bucketName)].Keys[string(key)] = value

	return nil
}

func (ms *MemStorage) Delete(bucketName []byte, key []byte) error {
	_, ok := ms.Buckets[string(bucketName)]
	if ok {
		delete(ms.Buckets[string(bucketName)].Keys, string(key))
	}
	return nil
}

func (ms *MemStorage) Get(bucketName []byte, key []byte) ([]byte, error) {
	bucket := ms.Buckets[string(bucketName)]
	bucket.lock.RLock()
	defer bucket.lock.RUnlock()

	val, ok := bucket.Keys[string(key)]
	if ok {
		return val, nil
	}
	return nil, nil
}

func (ms *MemStorage) GetKeys(bucketName []byte) ([][]byte, error) {
	bucket := ms.Buckets[string(bucketName)]

	bucket.lock.RLock()
	holder := make(map[string][]byte)
	for k, v := range bucket.Keys {
		holder[k] = v
	}
	bucket.lock.RUnlock()

	keys := make([][]byte, len(holder))
	i := 0
	for k := range holder {
		keys[i] = []byte(k)
		i++
	}
	return keys, nil
}

func (ms *MemStorage) ForEach(bucketName []byte, iterator func(k, v []byte) error) error {
	var err error

	bucket, ok := ms.Buckets[string(bucketName)]
	if !ok {
		log.Crit("unknown bucket", "bucket", string(bucketName))
	}

	bucket.lock.RLock()
	holder := make(map[string][]byte)
	for k, v := range bucket.Keys {
		holder[k] = v
	}
	bucket.lock.RUnlock()

	for k, v := range holder {
		err = iterator([]byte(k), v)
		if err != nil {
			break
		}
	}

	return err
}
