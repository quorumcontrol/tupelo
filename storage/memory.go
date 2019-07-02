package storage

import (
	"github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
)

// NewDefaultMemory returns a mutex-wrapped NewMap datastore
// for use in non-persistent situations.
func NewDefaultMemory() datastore.Batching {
	return dsync.MutexWrap(datastore.NewMapDatastore())
}
