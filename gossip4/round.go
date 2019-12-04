package gossip4

import "github.com/ipfs/go-hamt-ipld"

type round struct {
	snowball *Snowball
	height   uint64
	state    *hamt.Node
}

func newRound(height uint64) *round {
	return &round{
		height:   height,
		snowball: NewSnowball(0.8, 150, 10), // 2 is really low, should be 10 but this is just testing
		//TODO: should there be a state here? I think it should only be here after it's finalized
	}
}
