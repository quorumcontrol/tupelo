package wallet

import (
	"crypto/ecdsa"
	"github.com/quorumcontrol/chaintree/chaintree"
)

type Wallet interface {
	GetChain(id string) (*chaintree.ChainTree, error)
	SaveChain(chain *chaintree.ChainTree) error
	//Balances(id string) (map[string]uint64, error)
	GetChainIds() ([]string, error)
	Close()

	GetKey(addr string) (*ecdsa.PrivateKey, error)
	GenerateKey() (*ecdsa.PrivateKey, error)
	ListKeys() ([]string, error)
}
