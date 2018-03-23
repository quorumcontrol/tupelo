package wallet

import "github.com/quorumcontrol/qc3/consensus/consensuspb"

type Wallet interface {
	GetChain(id string) (*consensuspb.Chain, error)
	SetChain(id string, chain *consensuspb.Chain) (error)
	GetChainIds() ([]string,error)
	Close()
}
