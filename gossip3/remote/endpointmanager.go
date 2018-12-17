package remote

import (
	"crypto/ecdsa"
	"fmt"
	"strings"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/tupelo/p2p"
)

const routableSeparator = "-"

type routableAddress string

func NewRoutableAddress(from, to string) routableAddress {
	return routableAddress(from + routableSeparator + to)
}

func (ra routableAddress) From() string {
	return strings.Split(string(ra), routableSeparator)[0]
}

func (ra routableAddress) To() string {
	return strings.Split(string(ra), routableSeparator)[1]
}

func (ra routableAddress) Swap() routableAddress {
	split := strings.Split(string(ra), routableSeparator)
	return routableAddress(split[1] + routableSeparator + split[0])
}

func (ra routableAddress) String() string {
	return string(ra)
}

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
	router, err := actor.SpawnNamed(newRouterProps(host), "router-"+host.Identity())
	if err != nil {
		panic(fmt.Sprintf("error spawning router: %v", err))
	}
	globalManager.gateways[host.Identity()] = router
	return router
}

func RegisterBridge(from string, to *ecdsa.PublicKey) {
	router, ok := globalManager.gateways[from]
	if !ok {
		panic(fmt.Sprintf("router not found: %s", from))
	}
	router.Tell(&internalCreateBridge{from: from, to: to})
}

func remoteHandler(pid *actor.PID) (actor.Process, bool) {
	from := routableAddress(pid.Address).From()
	for gateway, router := range globalManager.gateways {
		if from == gateway {
			ref := newProcess(pid, router)
			return ref, true
		}
	}
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
