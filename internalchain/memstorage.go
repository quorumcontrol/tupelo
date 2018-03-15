package internalchain

import "github.com/quorumcontrol/qc3/consensus/consensuspb"

type MemStorage struct {
	Chains map[string]*consensuspb.ChainTip
}

func NewMemStorage() (*MemStorage) {
	return &MemStorage{
		Chains: make(map[string]*consensuspb.ChainTip),
	}
}

func (ms *MemStorage) Set(id string, chainTip *consensuspb.ChainTip) error {
	ms.Chains[id] = chainTip
	return nil
}

func (ms *MemStorage) Get(id string) (*consensuspb.ChainTip,error){
	ch,ok :=  ms.Chains[id]
	if ok {
		return ch, nil
	} else {
		return &consensuspb.ChainTip{
			Id: id,
		}, nil
	}
}