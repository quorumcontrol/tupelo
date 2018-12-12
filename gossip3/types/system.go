package types

import (
	"crypto/rand"
	"math/big"

	"github.com/AsynkronIT/protoactor-go/actor"
)

type System struct {
	NotaryGroup *NotaryGroup
	Syncers     *actor.PIDSet
}

func NewSystem(ng *NotaryGroup) *System {
	s := &System{
		NotaryGroup: ng,
		Syncers:     actor.NewPIDSet(),
	}
	for _, signer := range ng.Signers {
		s.Syncers.Add(signer.Actor)
	}
	return s
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
