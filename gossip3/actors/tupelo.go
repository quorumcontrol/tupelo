package actors

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/types"
)

const mempoolKind = "mempool"
const committedKind = "committed"

// TupeloNode is the main logic of the entire system,
// consisting of multiple gossipers
type TupeloNode struct {
	middleware.LogAwareHolder

	self              *types.Signer
	notaryGroup       *types.NotaryGroup
	mempoolGossiper   *actor.PID
	committedGossiper *actor.PID

	currentStateStore *actor.PID
	mempoolStore      *actor.PID
	committedStore    *actor.PID
}

func NewTupeloNodeProps(self *types.Signer, ng *types.NotaryGroup) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &TupeloNode{
			self:        self,
			notaryGroup: ng,
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (tn *TupeloNode) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		tn.handleStarted(context)
	case *messages.GetTip:
		tn.handleGetTip(context, msg)
	case *messages.GetSyncer:
		tn.handleGetSyncer(context, msg)
	}
}

func (tn *TupeloNode) handleGetTip(context actor.Context, msg *messages.GetTip) {
	tn.currentStateStore.Request(&messages.Get{Key: msg.ObjectID}, context.Sender())
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

func (tn *TupeloNode) handleStarted(context actor.Context) {
	currentStateStore, err := context.SpawnNamed(NewStorageProps(), "currentStateStore")
	if err != nil {
		panic(fmt.Sprintf("err: %v", err))
	}

	mempoolStore, err := context.SpawnNamed(NewStorageProps(), "mempoolvalidator")
	if err != nil {
		panic(fmt.Sprintf("err: %v", err))
	}
	// TODO: this should be a different validator
	committedStore, err := context.SpawnNamed(NewStorageProps(), "committedvalidator")
	if err != nil {
		panic(fmt.Sprintf("err: %v", err))
	}

	mempoolPusherProps := NewPushSyncerProps(mempoolKind, mempoolStore)
	committedProps := NewPushSyncerProps(committedKind, committedStore)

	mempoolGossiper, err := context.SpawnNamed(NewGossiperProps(mempoolKind, mempoolStore, tn.notaryGroup, mempoolPusherProps), mempoolKind)
	if err != nil {
		panic(fmt.Sprintf("error spawning mempool: %v", err))
	}
	committedGossiper, err := context.SpawnNamed(NewGossiperProps(committedKind, committedStore, tn.notaryGroup, committedProps), committedKind)
	if err != nil {
		panic(fmt.Sprintf("error spawning mempool: %v", err))
	}
	tn.mempoolGossiper = mempoolGossiper
	tn.committedGossiper = committedGossiper
	tn.currentStateStore = currentStateStore
}
