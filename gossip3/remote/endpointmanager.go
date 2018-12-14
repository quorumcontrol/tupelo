package remote

import (
	"fmt"
	"log"

	"github.com/AsynkronIT/protoactor-go/actor"
	pnet "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/tinylib/msgp/msgp"
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
	sm := &remoteManger{
		streamer: streamer,
	}

	host.SetStreamHandler(p2pProtocol, sm.streamHandler)

	return sm
}

func (rm *remoteManger) streamHandler(s pnet.Stream) {

}

func (rm *remoteManger) remoteDeliver(rd *remoteDeliver) {
	_, ok := rd.message.(msgp.Marshaler)
	if !ok {
		log.Printf("cannot send: %v", rd.message)
		return
	}
	wd := ToWireDelivery(rd)
	wd.Outgoing = true
	rm.streamer.Tell(wd)
}

func (rm *remoteManger) stop() {
	rm.streamer.GracefulStop()
}
