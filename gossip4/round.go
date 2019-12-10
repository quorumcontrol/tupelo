package gossip4

import "github.com/ipfs/go-hamt-ipld"

const (
	defaultAlpha = 0.8
	defaultBeta  = 150
	defaultK     = 10
)

type round struct {
	snowball *Snowball
	height   uint64
	state    *hamt.Node
}

func newRound(height uint64) *round {
	return &round{
		height:   height,
		snowball: NewSnowball(defaultAlpha, defaultBeta, defaultK),
	}
}
