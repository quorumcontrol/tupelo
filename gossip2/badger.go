package gossip2

import (
	"log"
	"runtime"

	"github.com/dgraph-io/badger"
)

func init() {
	// Badger recommends setting this to 128 or higher
	// Defaults to 12 on my laptop
	runtime.GOMAXPROCS(128)
}

type BadgerStorage struct {
	db *badger.DB
}

func NewBadgerStorage(path string) *BadgerStorage {
	opts := badger.DefaultOptions
	opts.Dir = path
	opts.ValueDir = path

	db, err := badger.Open(opts)
	if err != nil {
		log.Printf("error opening db %v", err)
	}

	return &BadgerStorage{db: db}
}

func (bs *BadgerStorage) Close() {
	bs.db.Close()
}

func (bs *BadgerStorage) Set(key []byte, value []byte) error {
	return bs.db.Update(func(txn *badger.Txn) error {
		return txn.Set(key, value)
	})
}

func (bs *BadgerStorage) Delete(key []byte) error {
	return bs.db.Update(func(txn *badger.Txn) error {
		return txn.Delete(key)
	})
}

func (bs *BadgerStorage) Get(key []byte) ([]byte, error) {
	var valueBytes []byte

	err := bs.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(key)
		if err != nil {
			return err
		}
		valueBytes, err = item.ValueCopy(valueBytes)
		return err
	})

	switch err {
	// Don't treat missing key as an error
	case badger.ErrKeyNotFound:
		return valueBytes, nil
	default:
		return valueBytes, err
	}
}

func (bs *BadgerStorage) GetAll(keys [][]byte) ([]KeyValuePair, error) {
	pairs := make([]KeyValuePair, len(keys))

	err := bs.db.View(func(txn *badger.Txn) error {
		for i, key := range keys {
			item, err := txn.Get(key)
			if err != nil && err != badger.ErrKeyNotFound {
				return err
			}
			val, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}

			pairs[i] = KeyValuePair{Key: item.KeyCopy(nil), Value: val}
		}
		return nil
	})

	return pairs, err
}

func (bs *BadgerStorage) Exists(key []byte) (bool, error) {
	err := bs.db.View(func(txn *badger.Txn) error {
		log.Printf("getting %s", key)
		_, err := txn.Get(key)
		return err
	})

	switch err {
	// Don't treat missing key as an error
	case badger.ErrKeyNotFound:
		return false, nil
	case nil:
		return true, nil
	default:
		return false, err
	}
}

type KeyValuePair struct {
	Key   []byte
	Value []byte
}

func (bs *BadgerStorage) GetPairsByPrefix(prefix []byte) ([]KeyValuePair, error) {
	var pairs []KeyValuePair

	err := bs.db.View(func(txn *badger.Txn) error {
		return bucketIterator(txn, badger.DefaultIteratorOptions, prefix, func(key []byte, item *badger.Item) error {
			val, err := item.ValueCopy(nil)
			if err != nil {
				return err
			}
			pairs = append(pairs, KeyValuePair{Key: key, Value: val})
			return nil
		})
	})
	return pairs, err
}

func (bs *BadgerStorage) GetKeysByPrefix(prefix []byte) ([][]byte, error) {
	var keys [][]byte

	err := bs.db.View(func(txn *badger.Txn) error {
		opts := badger.DefaultIteratorOptions
		opts.PrefetchValues = false
		return bucketIterator(txn, opts, prefix, func(key []byte, item *badger.Item) error {
			keys = append(keys, key)
			return nil
		})
	})

	return keys, err
}

// func (bs *BadgerStorage) GetKeys() ([][]byte, error) {
// 	var keys [][]byte

// 	err := bs.db.View(func(txn *badger.Txn) error {
// 		opts := badger.DefaultIteratorOptions
// 		opts.PrefetchValues = false
// 		return bucketIterator(txn, opts, bucketName, func(key []byte, item *badger.Item) error {
// 			keys = append(keys, key)
// 			return nil
// 		})
// 	})

// 	return keys, err
// }

// ForEach executes a callback over every key/value inside a bucket
// allocations are only valid in the scope of the callback
func (bs *BadgerStorage) ForEach(prefix []byte, iterator func(k, v []byte) error) error {
	return bs.db.View(func(txn *badger.Txn) error {
		return bucketIterator(txn, badger.DefaultIteratorOptions, prefix, func(key []byte, item *badger.Item) error {
			val, err := item.Value()
			if err != nil {
				return err
			}
			return iterator(key, val)
		})
	})
}

func bucketIterator(txn *badger.Txn, options badger.IteratorOptions, prefix []byte, iterator func(key []byte, item *badger.Item) error) error {
	it := txn.NewIterator(options)
	defer it.Close()
	for it.Seek(prefix); it.ValidForPrefix(prefix); it.Next() {
		item := it.Item()
		key := item.KeyCopy(nil)
		err := iterator(key, item)
		if err != nil {
			return err
		}
	}
	return nil
}
