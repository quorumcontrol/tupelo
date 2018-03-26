package storage

type MemBucket struct {
	Keys map[string][]byte
}

type MemStorage struct {
	Buckets map[string]*MemBucket
}

var _ Storage = (*MemStorage)(nil)

func NewMemStorage() *MemStorage {
	return &MemStorage{
		Buckets: make(map[string]*MemBucket),
	}
}

func (ms *MemStorage) Close() {
	//noop
}


func (ms *MemStorage) CreateBucketIfNotExists(bucketName []byte) error {
	_,ok := ms.Buckets[string(bucketName)]
	if !ok {
		ms.Buckets[string(bucketName)] = &MemBucket{
			Keys: make(map[string][]byte),
		}
	}
	return nil
}

func (ms *MemStorage) Set(bucketName []byte, key []byte, value []byte) error {
	ms.Buckets[string(bucketName)].Keys[string(key)] = value
	return nil
}

func (ms *MemStorage) Delete(bucketName []byte, key []byte) error {
	delete(ms.Buckets[string(bucketName)].Keys, string(key))
	return nil
}

func (ms *MemStorage) Get(bucketName []byte, key []byte) ([]byte, error) {
	val,ok := ms.Buckets[string(bucketName)].Keys[string(key)]
	if ok {
		return val, nil
	}
	return nil, nil
}

func (ms *MemStorage) GetKeys(bucketName []byte) ([][]byte,error) {
	keys := make([][]byte, len(ms.Buckets[string(bucketName)].Keys))
	i := 0
	for k := range ms.Buckets[string(bucketName)].Keys {
		keys[i] = []byte(k)
		i++
	}
	return keys,nil
}

func (ms *MemStorage) ForEach(bucketName []byte, iterator func(k,v []byte) error) error {
	var err error
	for k, v := range ms.Buckets[string(bucketName)].Keys {
		err = iterator([]byte(k),v)
		if err != nil {
			break
		}
	}

	return err
}
