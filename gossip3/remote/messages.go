//go:generate msgp

package remote

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
)

type msgPackObject interface {
	MarshalMsg([]byte) ([]byte, error)
	UnmarshalMsg([]byte) ([]byte, error)
}

type remoteDeliver struct {
	header       actor.ReadonlyMessageHeader
	message      interface{}
	target       *actor.PID
	sender       *actor.PID
	serializerID int32
}

func ToWireDelivery(rd *remoteDeliver) *WireDelivery {
	marshaled, err := rd.message.(msgPackObject).MarshalMsg(nil)
	if err != nil {
		panic(fmt.Errorf("could not marshal message: %v", err))
	}
	return &WireDelivery{
		Header:  rd.header.ToMap(),
		Message: marshaled,
		Target:  ToActorPid(rd.target),
		Sender:  ToActorPid(rd.sender),
	}
}

type ActorPID struct {
	Address string
	Id      string
}

func ToActorPid(a *actor.PID) *ActorPID {
	return &ActorPID{
		Address: a.Address,
		Id:      a.Id,
	}
}

func FromActorPid(a *ActorPID) *actor.PID {
	return actor.NewPID(a.Address, a.Id)
}

type WireDelivery struct {
	Header  map[string]string
	Message []byte
	Target  *ActorPID
	Sender  *ActorPID
}
