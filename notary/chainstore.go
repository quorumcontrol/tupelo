package notary

import (
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/gogo/protobuf/proto"
	"fmt"
)

//Set(id string, chain *consensuspb.ChainTip) (error)
//Get(id string) (*consensuspb.ChainTip,error)

type ChainStore struct {
	storage storage.Storage
	bucketName []byte
}

var _ ChainStorage = (*ChainStore)(nil)

func NewChainStore(bucketName string, store storage.Storage) *ChainStore {
	store.CreateBucketIfNotExists([]byte(bucketName))
	return &ChainStore{
		storage: store,
		bucketName: []byte(bucketName),
	}
}

func (cs *ChainStore) Set(id string, chain *consensuspb.ChainTip) error {
	chainBytes,err := proto.Marshal(chain)
	if err != nil {
		return fmt.Errorf("error marshaling: %v", err)
	}

	return cs.storage.Set(cs.bucketName, []byte(id), chainBytes)
}

func (cs *ChainStore) Get(id string) (*consensuspb.ChainTip,error) {
	chainBytes,err := cs.storage.Get(cs.bucketName, []byte(id))
	if err != nil {
		return nil, fmt.Errorf("error getting: %v", err)
	}

	if len(chainBytes) == 0 || chainBytes == nil {
		return &consensuspb.ChainTip{
			Id: id,
		},nil
	}

	tip := &consensuspb.ChainTip{}
	err = proto.Unmarshal(chainBytes, tip)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling: %v", err)
	}

	return tip,nil
}
