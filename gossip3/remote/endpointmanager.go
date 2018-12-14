package remote

import (
	"fmt"
	"log"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/tupelo/p2p"
)

var endpointManager *remoteManger

type remoteManger struct {
	streamer *actor.PID
}

func newRemoteManager(host p2p.Node) *remoteManger {
	streamer, err := actor.SpawnNamed(newStreamerProps(host), "streamer")
	if err != nil {
		panic(fmt.Sprintf("error spawning streamer: %v", err))
	}
	return &remoteManger{
		streamer: streamer,
	}
}

func (rm *remoteManger) remoteDeliver(rd *remoteDeliver) {
	log.Printf("rd: %v", rd)
	rm.streamer.Tell(ToWireDelivery(rd))
}

func (rm *remoteManger) stop() {
	rm.streamer.GracefulStop()
}
