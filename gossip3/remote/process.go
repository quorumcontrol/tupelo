package remote

import (
	"log"
	"reflect"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/tinylib/msgp/msgp"
)

type process struct {
	pid     *actor.PID
	gateway *actor.PID
}

func newProcess(pid, gateway *actor.PID) actor.Process {
	return &process{
		pid:     pid,
		gateway: gateway,
	}
}

func (ref *process) SendUserMessage(pid *actor.PID, message interface{}) {
	header, msg, sender := actor.UnwrapEnvelope(message)
	SendMessage(ref.gateway, pid, header, msg, sender, -1)
}

func SendMessage(gateway, pid *actor.PID, header actor.ReadonlyMessageHeader, message interface{}, sender *actor.PID, serializerID int32) {
	rd := &remoteDeliver{
		header:       header,
		message:      message,
		sender:       sender,
		target:       pid,
		serializerID: serializerID,
	}

	_, ok := rd.message.(msgp.Marshaler)
	if !ok {
		log.Printf("cannot send: %s", reflect.TypeOf(rd.message))
		return
	}
	wd := ToWireDelivery(rd)
	wd.Outgoing = true
	gateway.Tell(wd)
}

func (ref *process) SendSystemMessage(pid *actor.PID, message interface{}) {

	//intercept any Watch messages and direct them to the endpoint manager
	switch msg := message.(type) {
	case *actor.Watch:
		panic("remote watching unsupported")
		// rw := &remoteWatch{
		// 	Watcher: msg.Watcher,
		// 	Watchee: pid,
		// }
		// endpointManager.remoteWatch(rw)
	case *actor.Unwatch:
		panic("remote unwatching unsupported")
		// ruw := &remoteUnwatch{
		// 	Watcher: msg.Watcher,
		// 	Watchee: pid,
		// }
		// endpointManager.remoteUnwatch(ruw)
	default:
		SendMessage(ref.gateway, pid, nil, msg, nil, -1)
	}
}

func (ref *process) Stop(pid *actor.PID) {
	panic("remote stop is unsupported")
}

func (ref *process) remoteDeliver(rd *remoteDeliver) {
	_, ok := rd.message.(msgp.Marshaler)
	if !ok {
		log.Printf("cannot send: %v", rd.message)
		return
	}
	wd := ToWireDelivery(rd)
	wd.Outgoing = true
	ref.gateway.Tell(wd)
}
