package messages

import (
	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/differencedigest/ibf"
)

type Store struct {
	Key   []byte
	Value []byte
}

type Remove struct {
	Key []byte
}

type GetStrata struct{}

type GetStorage struct{}
type GetSyncer struct{}

type GetPrefix struct {
	Prefix []byte
}

type system interface {
	GetRandomSyncer() *actor.PID
}
type StartGossip struct {
	System system
}

type GetIBF struct {
	Size int
}

type DoPush struct {
	RemoteSyncer *actor.PID
}

type ProvideStrata struct {
	Strata    ibf.DifferenceStrata
	Validator *actor.PID
}

type ProvideBloomFilter struct {
	Filter    ibf.InvertibleBloomFilter
	Validator *actor.PID
}

type RequestIBF struct {
	Count  int
	Result *ibf.DecodeResults
}

type RequestKeys struct {
	Keys []uint64
}

type SendPrefix struct {
	Prefix      []byte
	Destination *actor.PID
}
