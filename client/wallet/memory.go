package wallet

import (
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"crypto/ecdsa"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
)

type MemoryWallet struct {
	Id string
	chains map[string]*consensuspb.Chain
	keys map[string]*ecdsa.PrivateKey
}

// just make sure that implementation conforms to the interface
var _ Wallet = (*MemoryWallet)(nil)

func NewMemoryWallet(id string) *MemoryWallet {
	return &MemoryWallet{
		Id: id,
		chains: make(map[string]*consensuspb.Chain),
		keys: make(map[string]*ecdsa.PrivateKey),
	}
}

func (mw *MemoryWallet) GetChain(id string) (*consensuspb.Chain, error) {
	chain,ok := mw.chains[id]
	if ok {
		return chain,nil
	}
	return nil,nil
}

func (mw *MemoryWallet) SetChain(id string, chain *consensuspb.Chain) (error) {
	mw.chains[id] = chain
	return nil
}

func (mw *MemoryWallet) GetChainIds() ([]string,error) {
	ids := make([]string, len(mw.chains))
	i := 0
	for k := range mw.chains {
		ids[i] = k
		i++
	}
	return ids, nil
}

func (mw *MemoryWallet) Close() {
	return // just fulfilling the interface
}


func (mw *MemoryWallet) GetKey(addr string) (*ecdsa.PrivateKey,error) {
	return mw.keys[addr], nil
}

func (mw *MemoryWallet) GenerateKey() (*ecdsa.PrivateKey,error) {
	key,err := crypto.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("error generating key: %v", err)
	}

	mw.keys[crypto.PubkeyToAddress(key.PublicKey).String()] = key

	return key,nil
}

func (mw *MemoryWallet) ListKeys() ([]string, error) {
	addrs := make([]string, len(mw.keys))
	i := 0
	for addr := range mw.keys {
		addrs[i] = addr
	}
	return addrs, nil
}
