package storage

import (
	"os"
	"strings"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	datastore "github.com/ipfs/go-datastore"
	dsbadger "github.com/ipfs/go-ds-badger"
)

func NewDefaultBadger(path string) (datastore.Batching, error) {
	opts := badger.DefaultOptions("")
	opts.Dir = path
	opts.ValueDir = path

	lowMemoryModeVal, lowMemoryModeSet := os.LookupEnv("BADGERDB_LOW_MEMORY_MODE")
	if lowMemoryModeSet && strings.ToLower(lowMemoryModeVal) != "false" {
		opts.ValueLogLoadingMode = options.FileIO
		opts.TableLoadingMode = options.FileIO
	}

	return dsbadger.NewDatastore(path, &dsbadger.Options{Options: opts})
}
