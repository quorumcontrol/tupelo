package remote

import (
	"crypto/ecdsa"
	"fmt"
	"log"

	"github.com/AsynkronIT/protoactor-go/actor"
	peer "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-peer"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/tinylib/msgp/msgp"
)

type actorRegistry map[string]*actor.PID

type remoteManger struct {
	router *actor.PID
}

// These are GLOBAL state used to handle the singleton for routing to remote hosts
// and the registry of local bridges
var endpointManager *remoteManger
var incomingHandlerRegistry = make(actorRegistry)

func Start() {
	endpointManager = newRemoteManager()
	actor.ProcessRegistry.RegisterAddressResolver(remoteHandler)
}

func Stop() {
	endpointManager.stop()
	for _, bridge := range incomingHandlerRegistry {
		bridge.Poison()
	}
	endpointManager = nil
	incomingHandlerRegistry = make(actorRegistry)
}

func Register(peer peer.ID, handler *actor.PID) {
	endpointManager.router.Tell(&registerBridge{Peer: peer.Pretty(), Handler: handler})
}

func NewBridge(remoteKey *ecdsa.PublicKey, host p2p.Node) *actor.PID {
	peer, err := p2p.PeerFromEcdsaKey(remoteKey)
	if err != nil {
		panic(fmt.Sprintf("error getting peer from key: %v", err))
	}

	bridge, err := actor.SpawnNamed(newBridgeProps(host, remoteKey), "bridge-"+peer.Pretty())
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}
	Register(peer, bridge)
	return bridge
}

func remoteHandler(pid *actor.PID) (actor.Process, bool) {
	ref := newProcess(pid)
	return ref, true
}

func newRemoteManager() *remoteManger {
	streamer, err := actor.SpawnNamed(newRouterProps(), "remote-router")
	if err != nil {
		panic(fmt.Sprintf("error spawning streamer: %v", err))
	}
	rm := &remoteManger{
		router: streamer,
	}

	return rm
}

func (rm *remoteManger) remoteDeliver(rd *remoteDeliver) {
	_, ok := rd.message.(msgp.Marshaler)
	if !ok {
		log.Printf("cannot send: %v", rd.message)
		return
	}
	wd := ToWireDelivery(rd)
	wd.Outgoing = true
	rm.router.Tell(wd)
}

func (rm *remoteManger) stop() {
	rm.router.GracefulStop()
}
