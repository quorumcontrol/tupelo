package remote

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/p2p"
)

type actorRegistry map[string]*actor.PID

type remoteManger struct {
	gateways actorRegistry
}

// These are GLOBAL state used to handle the singleton for routing to remote hosts
// and the registry of local bridges
var globalManager *remoteManger

func Start() {
	globalManager = newRemoteManager()
	actor.ProcessRegistry.RegisterAddressResolver(remoteHandler)
}

func Stop() {
	globalManager.stop()
	globalManager = nil
}

func NewRouter(host p2p.Node) *actor.PID {
	middleware.Log.Infow("registering router", "host", host.Identity())
	router, err := actor.SpawnNamed(newRouterProps(host), "router-"+host.Identity())
	if err != nil {
		panic(fmt.Sprintf("error spawning router: %v", err))
	}
	globalManager.gateways[host.Identity()] = router
	return router
}

// func RegisterBridge(from string, to *ecdsa.PublicKey) {
// 	router, ok := globalManager.gateways[from]
// 	if !ok {
// 		panic(fmt.Sprintf("router not found: %s", from))
// 	}
// 	router.Tell(&internalCreateBridge{from: from, to: to})
// }

func remoteHandler(pid *actor.PID) (actor.Process, bool) {
	from := types.RoutableAddress(pid.Address).From()
	for gateway, router := range globalManager.gateways {
		if from == gateway {
			ref := newProcess(pid, router)
			return ref, true
		}
	}
	middleware.Log.Errorw("unhandled remote pid", "addr", pid.Address, "current", globalManager.gateways)
	panic(fmt.Sprintf("unhandled remote pid: %s id: %s", pid.Address, pid.GetId()))
	return nil, false
}

func newRemoteManager() *remoteManger {
	rm := &remoteManger{
		gateways: make(actorRegistry),
	}

	return rm
}

func (rm *remoteManger) stop() {
	for _, router := range rm.gateways {
		router.GracefulStop()
	}
}
