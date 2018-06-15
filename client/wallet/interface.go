package wallet

import (
	"crypto/ecdsa"

	"github.com/quorumcontrol/qc3/consensus/consensuspb"
)

type Wallet interface {
	GetChain(id string) (*consensuspb.Chain, error)
	SetChain(id string, chain *consensuspb.Chain) error
	Balances(id string) (map[string]uint64, error)
	GetChainIds() ([]string, error)
	Close()

	GetKey(addr string) (*ecdsa.PrivateKey, error)
	GenerateKey() (*ecdsa.PrivateKey, error)
	ListKeys() ([]string, error)
}
