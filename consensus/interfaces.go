package consensus

import "crypto/ecdsa"

type Wallet interface {
	GetChain(id string) (*SignedChainTree, error)
	SaveChain(chain *SignedChainTree) error
	//Balances(id string) (map[string]uint64, error)
	GetChainIds() ([]string, error)
	Close()

	GetKey(addr string) (*ecdsa.PrivateKey, error)
	GenerateKey() (*ecdsa.PrivateKey, error)
	ListKeys() ([]string, error)
}
