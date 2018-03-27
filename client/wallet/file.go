package wallet

import (
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"fmt"
	"github.com/gogo/protobuf/proto"
	"github.com/quorumcontrol/qc3/storage"
	"crypto/ecdsa"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/common"
)

var chainBucket = []byte("chains")
var keyBucket = []byte("keys")

// just make sure that implementation conforms to the interface
var _ Wallet = (*FileWallet)(nil)

type FileWallet struct {
	ebs storage.EncryptedStorage
}

func NewFileWallet(passphrase, path string) *FileWallet {
	ebs := storage.NewEncryptedBoltStorage(path)
	ebs.Unlock(passphrase)
	ebs.CreateBucketIfNotExists(chainBucket)
	ebs.CreateBucketIfNotExists(keyBucket)
	return &FileWallet{
		ebs: ebs,
	}
}

func (fw *FileWallet) Close() {
	fw.ebs.Close()
}

func (fw *FileWallet) GetChain(id string) (*consensuspb.Chain, error) {
	chainBytes,err := fw.ebs.Get(chainBucket, []byte(id))
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

func (fw *FileWallet) SetChain(id string, chain *consensuspb.Chain) (error) {
	chainBytes,err := proto.Marshal(chain)
	if err != nil {
		return fmt.Errorf("error marshaling: %v", err)
	}
	err = fw.ebs.Set(chainBucket, []byte(id), chainBytes)
	if err != nil {
		return fmt.Errorf("error saving: %v", err)
	}

	return nil
}

func (fw *FileWallet) GetChainIds() ([]string,error) {
	keys,err := fw.ebs.GetKeys(chainBucket)
	if err != nil {
		return nil, fmt.Errorf("error getting keys; %v", err)
	}
	ids := make([]string, len(keys))
	for i,k := range keys {
		ids[i] = string(k)
	}
	return ids, nil
}

func (fw *FileWallet) GetKey(addr string) (*ecdsa.PrivateKey,error) {
	keyBytes,err := fw.ebs.Get(keyBucket, common.HexToAddress(addr).Bytes())
	if err != nil {
		return nil, fmt.Errorf("error getting key: %v", err)
	}
	return crypto.ToECDSA(keyBytes)
}

func (fw *FileWallet) GenerateKey() (*ecdsa.PrivateKey,error) {
	key,err := crypto.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("error generating key: %v", err)
	}

	err = fw.ebs.Set(keyBucket, crypto.PubkeyToAddress(key.PublicKey).Bytes(), crypto.FromECDSA(key))
	if err != nil {
		return nil, fmt.Errorf("error generating key: %v", err)
	}

	return key,nil
}

func (fw *FileWallet) ListKeys() ([]string, error) {
	keys,err := fw.ebs.GetKeys(keyBucket)
	if err != nil {
		return nil, fmt.Errorf("error getting keys; %v", err)
	}
	addrs := make([]string, len(keys))
	for i,k := range keys {
		addrs[i] = common.BytesToAddress(k).String()
	}
	return addrs, nil
}
