package gossip3

import (
	"crypto/rand"
	"math/big"

	"github.com/AsynkronIT/protoactor-go/actor"
)

type System struct {
	Syncers *actor.PIDSet
}

func NewSystem() *System {
	return &System{
		Syncers: actor.NewPIDSet(),
	}
}

func (s *System) GetRandomSyncer() *actor.PID {
	vals := s.Syncers.Values()
	pid := vals[randInt(len(vals)-1)]
	return &pid
}

func randInt(max int) int {
	bigInt, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		panic("bad random")
	}
	return int(bigInt.Int64())
}
