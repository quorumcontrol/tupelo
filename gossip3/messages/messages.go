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

type GetSyncer struct {
	Kind string
}

type SyncDone struct{}

type ValidatorClear struct{}
type ValidatorWorking struct{}
type SubscribeValidatorWorking struct {
	Actor *actor.PID
}

type GetPrefix struct {
	Prefix []byte
}

type Get struct {
	Key []byte
}

type system interface {
	GetRandomSyncer() *actor.PID
}
type StartGossip struct {
	System system
}
type DoOneGossip struct{}

type GetIBF struct {
	Size int
}

type DoPush struct {
	System system
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

type RoundTransition struct {
	NextRound uint64
}
