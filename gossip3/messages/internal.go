//go:generate msgp

package messages

import (
	"github.com/quorumcontrol/differencedigest/ibf"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/serializer"
)

func init() {
	serializer.RegisterEncodable(GetSyncer{})
	serializer.RegisterEncodable(SyncDone{})
	serializer.RegisterEncodable(NoSyncersAvailable{})
	serializer.RegisterEncodable(SyncerAvailable{})
	serializer.RegisterEncodable(ProvideStrata{})
	serializer.RegisterEncodable(ProvideBloomFilter{})
	serializer.RegisterEncodable(RequestKeys{})
	serializer.RegisterEncodable(RequestIBF{})
}

type DestinationHolder struct {
	Destination *extmsgs.ActorPID
}

func (dh *DestinationHolder) SetDestination(newDestination *extmsgs.ActorPID) {
	dh.Destination = newDestination
}

func (dh *DestinationHolder) GetDestination() *extmsgs.ActorPID {
	return dh.Destination
}

type GetSyncer struct {
	Kind string
}

type RequestIBF struct {
	Count int
}

type SyncDone struct{}

type NoSyncersAvailable struct{}

type SyncerAvailable struct {
	DestinationHolder
}

type RequestKeys struct {
	Keys []uint64
}

type ProvideStrata struct {
	DestinationHolder

	Strata *ibf.DifferenceStrata
}

type ProvideBloomFilter struct {
	DestinationHolder

	Filter *ibf.InvertibleBloomFilter
}
