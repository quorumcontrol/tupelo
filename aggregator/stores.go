package aggregator

import(
	"github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
)

func NewMemoryStore() datastore.Batching {
	return dsync.MutexWrap(datastore.NewMapDatastore())
}
