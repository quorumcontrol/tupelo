package nodebuilder


import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/quorumcontrol/chaintree/cachedblockstore"

	s3ds "github.com/ipfs/go-ds-s3"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/pkg/errors"

	"github.com/dgraph-io/badger"
	"github.com/dgraph-io/badger/options"
	"github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	dsbadger "github.com/ipfs/go-ds-badger"
)

type HumanStorageConfig struct {
	Kind string
	Path string // for badger
	// CacheSize defaults to 100 (when set to 0), use -1 for no cache
	// only used for the blockstore
	CacheSize int

	// remaining are For s3
	RegionEndpoint string
	Bucket         string
	Region         string
	AccessKey      string
	SecretKey      string
	LocalS3        bool
	RootDirectory  string
}

func (hsc *HumanStorageConfig) ToDatastore() (datastore.Batching, error) {
	switch strings.ToLower(hsc.Kind) {
	case "", "memory": // not-specified means memory
		return dsync.MutexWrap(datastore.NewMapDatastore()), nil
	case "badger":
		return NewDefaultBadger(hsc.Path)
	case "s3":
		return NewS3(hsc)
	default:
		return nil, fmt.Errorf("error, unknown type: %s", hsc.Kind)
	}
}

func (hsc *HumanStorageConfig) ToBlockstore() (blockstore.Blockstore, error) {
	datastore, err := hsc.ToDatastore()
	if err != nil {
		return nil, fmt.Errorf("error getting datastore: %v", err)
	}
	bs := blockstore.NewBlockstore(datastore)
	bs = blockstore.NewIdStore(bs)

	// use -1 to turn off the cache,
	// using a 0 defaults to a 100 cacheSize
	// anything greater than 0 is used for the cacheSize
	if hsc.CacheSize >= 0 {
		cacheSize := 100
		if hsc.CacheSize > 0 {
			cacheSize = hsc.CacheSize
		}
		wrapped, err := cachedblockstore.WrapInCache(bs, cacheSize)
		if err != nil {
			return nil, fmt.Errorf("error wrapping: %v", err)
		}
		return wrapped, nil
	}
	return bs, nil
}

func NewS3(hsc *HumanStorageConfig) (datastore.Batching, error) {
	s3conf := s3ds.Config{
		RegionEndpoint: hsc.RegionEndpoint,
		Bucket:         hsc.Bucket,
		Region:         hsc.Region,
		AccessKey:      hsc.AccessKey,
		SecretKey:      hsc.SecretKey,
		RootDirectory:  hsc.RootDirectory,
	}

	ds, err := s3ds.NewS3Datastore(s3conf)
	if err != nil {
		return nil, errors.Wrap(err, "error creating datastore")
	}
	if hsc.LocalS3 {
		logger.Debugf("creating bucket")
		if err := devMakeBucket(ds.S3, hsc.Bucket); err != nil {
			return nil, errors.Wrap(err, "error creating bucket")
		}
	}
	return ds, nil
}

// NewDefaultBadger is a convenience function to produce our "standard"
// badger with the optional low memory mode
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

func devMakeBucket(s3obj *s3.S3, bucketName string) error {
	_, err := s3obj.CreateBucket(&s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	})
	// since this is local, we need to wait a sec before using it
	time.Sleep(1 * time.Second)

	return err
}
