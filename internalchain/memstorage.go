package internalchain

type MemStorage struct {
	Chains map[string]*InternalChain
}

func NewMemStorage() (*MemStorage) {
	return &MemStorage{
		Chains: make(map[string]*InternalChain),
	}
}

func (ms *MemStorage) Set(id string, chain *InternalChain) error {
	ms.Chains[id] = chain
	return nil
}

func (ms *MemStorage) Get(id string) (*InternalChain,error){
	ch,ok :=  ms.Chains[id]
	if ok {
		return ch, nil
	} else {
		return &InternalChain{
			Id: id,
		}, nil
	}
}