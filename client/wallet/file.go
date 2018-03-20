package wallet

import (
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/coreos/bbolt"
	"github.com/ethereum/go-ethereum/log"
	"fmt"
	"github.com/gogo/protobuf/proto"
)

var chainBucket = []byte("chains")
var keyBucket = []byte("keys")

type Filewallet struct {
	db *bolt.DB
	secretKey *[32]byte
}

func NewFileWallet(secretKey *[32]byte, path string) *Filewallet {
	db, err := bolt.Open(path, 0600, nil)
	if err != nil {
		log.Crit("error opening db", "error", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		_,err := tx.CreateBucketIfNotExists(chainBucket)
		if err != nil {
			return err
		}
		_,err = tx.CreateBucketIfNotExists(keyBucket)
		return err
	})
	if err != nil {
		log.Crit("error creating bucket", "error", err)
	}

}

func (fw *Filewallet) GetChain(id string) (*consensuspb.Chain, error) {
	var chainBytes []byte
	err := fw.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(chainBucket)
		chainBytes = b.Get([]byte(id))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("error getting: %v", err)
	}

	chain := &consensuspb.Chain{}
	err = proto.Unmarshal(chainBytes, chain)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling: %v", err)
	}

	return chain,nil
}

func (fw *Filewallet) SetChain(id string, chain *consensuspb.Chain) (error) {
	chainBytes,err := proto.Marshal(chain)
	if err != nil {
		return fmt.Errorf("error marshaling: %v", err)
	}
	err = fw.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(chainBucket)
		err := b.Put([]byte(id), chainBytes)
		return err
	})

	if err != nil {
		return fmt.Errorf("error saving: %v", err)
	}

	return nil
}

func (fw *Filewallet) GetChainIds() ([]string,error) {
	ids := make([]string, len(mw.chains))
	i := 0
	for k := range mw.chains {
		ids[i] = k
		i++
	}
	return ids, nil
}

//
//func (fs *FileStorage) Set(id string, chainTip *consensuspb.ChainTip) error {
//	tipBytes,err := proto.Marshal(chainTip)
//	if err != nil {
//		return fmt.Errorf("error marshaling: %v", err)
//	}
//	err = fs.db.Update(func(tx *bolt.Tx) error {
//		b := tx.Bucket(bucketName)
//		err := b.Put([]byte(id), tipBytes)
//		return err
//	})
//
//	if err != nil {
//		return fmt.Errorf("error saving: %v", err)
//	}
//
//	return nil
//}
//
//func (fs *FileStorage) Get(id string) (*consensuspb.ChainTip,error) {
//	var tipBytes []byte
//	err := fs.db.View(func(tx *bolt.Tx) error {
//		b := tx.Bucket(bucketName)
//		tipBytes = b.Get([]byte(id))
//		return nil
//	})
//	if err != nil {
//		return nil, fmt.Errorf("error getting: %v", err)
//	}
//
//	tip := &consensuspb.ChainTip{}
//	err = proto.Unmarshal(tipBytes, tip)
//	if err != nil {
//		return nil, fmt.Errorf("error unmarshaling: %v", err)
//	}
//
//	return tip,nil
//}
//
//
