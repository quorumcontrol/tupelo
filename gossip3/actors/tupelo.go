package actors

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
)

const mempoolKind = "mempool"
const committedKind = "committed"

// TupeloNode is the main logic of the entire system,
// consisting of multiple gossipers
type TupeloNode struct {
	mempoolGossiper   *actor.PID
	committedGossiper *actor.PID
}

func NewTupeloNodeProps() *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &TupeloNode{}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (tn *TupeloNode) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		tn.handleStarted(context)
	case *messages.GetSyncer:
		tn.handleGetSyncer(context, msg)
	case *messages.StartGossip:
		tn.handleStartGossip(context, msg)
	case *messages.Store:
		context.Forward(tn.mempoolGossiper)
	case *messages.Get:
		context.Forward(tn.mempoolGossiper)
	}
}

func (tn *TupeloNode) handleGetSyncer(context actor.Context, msg *messages.GetSyncer) {
	switch msg.Kind {
	case mempoolKind:
		context.Forward(tn.mempoolGossiper)
	case committedKind:
		context.Forward(tn.committedGossiper)
	default:
		panic("unknown gossiper")
	}
}

func (tn *TupeloNode) handleStartGossip(context actor.Context, msg *messages.StartGossip) {
	context.Forward(tn.mempoolGossiper)
	context.Forward(tn.committedGossiper)
}

func (tn *TupeloNode) handleStarted(context actor.Context) {
	mempoolGossiper, err := context.SpawnNamed(NewGossiperProps(mempoolKind, NewMemPoolValidatorProps, NewStorageProps()), mempoolKind)
	if err != nil {
		panic(fmt.Sprintf("error spawning mempool: %v", err))
	}
	// TODO: this should be a different validator
	committedGossiper, err := context.SpawnNamed(NewGossiperProps(committedKind, NewMemPoolValidatorProps, NewStorageProps()), committedKind)
	if err != nil {
		panic(fmt.Sprintf("error spawning mempool: %v", err))
	}
	tn.mempoolGossiper = mempoolGossiper
	tn.committedGossiper = committedGossiper
}
