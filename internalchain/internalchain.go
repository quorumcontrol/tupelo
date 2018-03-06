package internalchain

import "github.com/quorumcontrol/qc3/consensus/consensuspb"

type InternalChain struct {
	Id string
	LastBlock *consensuspb.Block
	CurrentOwners []*InternalOwnership
	MinimumOwners int
}

type InternalOwnership struct {
	Name string
	PublicKeys map[string]*consensuspb.PublicKey
}
