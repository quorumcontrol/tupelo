package internalchain

type MemStorage struct {
	Chains map[string]*InternalChain
}

func NewMemStorage() (*MemStorage) {
	return &MemStorage{
		Chains: make(map[string]*InternalChain),
	}
}

func (ms *MemStorage) Set(id string, chain *InternalChain) {
	ms.Chains[id] = chain
}

func (ms *MemStorage) Get(id string) (*InternalChain) {
	return ms.Chains[id]
}