package remote

import (
	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/p2p"
)

func Start(self *types.Signer, host p2p.Node) {
	endpointManager = newRemoteManager(host)

	actor.ProcessRegistry.RegisterAddressResolver(remoteHandler)
	actor.ProcessRegistry.Address = self.ActorAddress()
}

func Stop() {
	endpointManager.stop()
	endpointManager = nil
}
