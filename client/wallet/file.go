package wallet

import (
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"fmt"
	"github.com/gogo/protobuf/proto"
	"github.com/quorumcontrol/qc3/storage"
	"crypto/ecdsa"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/common"
	"reflect"
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

func (fw *FileWallet) Balances(id string) (map[string]uint64, error) {
	chain,err := fw.GetChain(id)
	if err != nil {
		return nil, fmt.Errorf("error getting chain: %v", err)
	}
	balances := make(map[string]uint64)

	for _,b := range chain.Blocks {
		for _,t := range b.SignableBlock.Transactions {
			if t.Type == consensuspb.RECEIVE_COIN {
				_typedReceived,_ := typedTransactionFrom(t, proto.MessageName(&consensuspb.ReceiveCoinTransaction{}))
				typedReceived := _typedReceived.(*consensuspb.ReceiveCoinTransaction)

				_typedSend,_ := typedTransactionFrom(typedReceived.SendTransaction, proto.MessageName(&consensuspb.SendCoinTransaction{}))
				typedSend := _typedSend.(*consensuspb.SendCoinTransaction)
				incBalance(balances, typedSend.Name, int64(typedSend.Amount))
			}
			if t.Type == consensuspb.MINT_COIN {
				_typed,_ := typedTransactionFrom(t, proto.MessageName(&consensuspb.MintCoinTransaction{}))
				typed := _typed.(*consensuspb.MintCoinTransaction)
				incBalance(balances, typed.Name, int64(typed.Amount))

			}
			if t.Type == consensuspb.SEND_COIN {
				_typed,_ := typedTransactionFrom(t, proto.MessageName(&consensuspb.SendCoinTransaction{}))
				typed := _typed.(*consensuspb.SendCoinTransaction)
				incBalance(balances, typed.Name, -1 * int64(typed.Amount))
			}
		}
	}
	return balances, nil
}

func typedTransactionFrom(transaction *consensuspb.Transaction, messageType string) (interface{},error) {
	instanceType := proto.MessageType(messageType)
	// instanceType will be a pointer type, so call Elem() to get the original Type and then interface
	// so that we can change it to the kind of object we want
	instance := reflect.New(instanceType.Elem()).Interface()
	err := proto.Unmarshal(transaction.Payload, instance.(proto.Message))
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling: %v", err)
	}
	return instance, nil
}

func incBalance(balances map[string]uint64, name string, amount int64) {
	balance,ok := balances[name]
	if ok {
		balances[name] = uint64(int64(balance) + amount)
	} else {
		balances[name] = uint64(amount)
	}
}