package notary

import (
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/coreos/bbolt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/golang/protobuf/proto"
	"fmt"
)

var bucketName = []byte("tips")

type FileStorage struct {
	db *bolt.DB
}

func NewFileStorage(path string) (*FileStorage) {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		log.Crit("error opening db", "error", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_,err := tx.CreateBucketIfNotExists(bucketName)
		return err
	})
	if err != nil {
		log.Crit("error creating bucket", "error", err)
	}

	return &FileStorage{
		db: db,
	}
}

func (fs *FileStorage) Set(id string, chainTip *consensuspb.ChainTip) error {
	tipBytes,err := proto.Marshal(chainTip)
	if err != nil {
		return fmt.Errorf("error marshaling: %v", err)
	}
	err = fs.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		err := b.Put([]byte(id), tipBytes)
		return err
	})

	if err != nil {
		return fmt.Errorf("error saving: %v", err)
	}

	return nil
}

func (fs *FileStorage) Get(id string) (*consensuspb.ChainTip,error) {
	var tipBytes []byte
	err := fs.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketName)
		tipBytes = b.Get([]byte(id))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error getting: %v", err)
	}

	tip := &consensuspb.ChainTip{}
	err = proto.Unmarshal(tipBytes, tip)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling: %v", err)
	}

	return tip,nil
}
