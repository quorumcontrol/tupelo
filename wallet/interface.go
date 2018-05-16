package wallet

import (
	"crypto/ecdsa"
	"github.com/quorumcontrol/qc3/consensus"
)

type Wallet interface {
	GetChain(id string) (*consensus.SignedChainTree, error)
	SaveChain(chain *consensus.SignedChainTree) error
	//Balances(id string) (map[string]uint64, error)
	GetChainIds() ([]string, error)
	Close()

	GetKey(addr string) (*ecdsa.PrivateKey, error)
	GenerateKey() (*ecdsa.PrivateKey, error)
	ListKeys() ([]string, error)
}
