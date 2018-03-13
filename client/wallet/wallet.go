package wallet

import "github.com/quorumcontrol/qc3/consensus/consensuspb"

type MemoryWallet struct {
	Id string
	chains map[string]*consensuspb.Chain
}

func NewMemoryWallet(id string) *MemoryWallet {
	return &MemoryWallet{
		Id: id,
		chains: make(map[string]*consensuspb.Chain),
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
