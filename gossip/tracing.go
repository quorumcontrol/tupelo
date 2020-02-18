package gossip

import (
	"github.com/ipfs/go-cid"
	g4services "github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo-go-sdk/tracing"
)

// AddBlockWrapper wraps an addblock request so that it can be traced through the system.
// currently exported so that the g3->g4 can use it
type AddBlockWrapper struct {
	tracing.ContextHolder
	*g4services.AddBlockRequest
	cid cid.Cid
}
