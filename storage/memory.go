package storage

import (
	"github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
)

func NewDefaultMemory() datastore.Batching {
	return dsync.MutexWrap(datastore.NewMapDatastore())
}
