package storage

import "github.com/quorumcontrol/storage"

type Reader interface {
	GetKeysByPrefix(prefix []byte) ([][]byte, error)
	GetPairsByPrefix(prefix []byte) ([]storage.KeyValuePair, error)
	Get(key []byte) ([]byte, error)
	Exists(key []byte) (bool, error)
}
