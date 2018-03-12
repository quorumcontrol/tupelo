package internalchain

import "github.com/quorumcontrol/qc3/consensus/consensuspb"

type MemStorage struct {
	Chains map[string]*consensuspb.Chain
}

func NewMemStorage() (*MemStorage) {
	return &MemStorage{
		Chains: make(map[string]*consensuspb.Chain),
	}
}

func (ms *MemStorage) Set(id string, chain *consensuspb.Chain) error {
	ms.Chains[id] = chain
	return nil
}

func (ms *MemStorage) Get(id string) (*consensuspb.Chain,error){
	ch,ok :=  ms.Chains[id]
	if ok {
		return ch, nil
	} else {
		return &consensuspb.Chain{
			Id: id,
		}, nil
	}
}