package gossip

import (
	"context"

	"github.com/AsynkronIT/protoactor-go/actor"
	format "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
)

type saveTransactionState struct {
	ctx        context.Context
	abrWrapper *AddBlockWrapper
}

type stateStorer struct {
	logger   logging.EventLogger
	dagStore nodestore.DagStore
}

func newStateStorer(logger logging.EventLogger, dagStore nodestore.DagStore) *stateStorer {
	return &stateStorer{
		logger:   logger,
		dagStore: dagStore,
	}
}

func (s *stateStorer) storeState(ctx context.Context, abrWrapper *AddBlockWrapper) {
	sw := safewrap.SafeWrap{}
	var stateNodes []format.Node

	abr := abrWrapper.AddBlockRequest

	for _, nodeBytes := range abr.State {
		stateNode := sw.Decode(nodeBytes)

		stateNodes = append(stateNodes, stateNode)
	}

	if sw.Err != nil {
		s.logger.Errorf("error decoding abr state: %v", sw.Err)
		return
	}

	err := s.dagStore.AddMany(ctx, stateNodes)
	if err != nil {
		s.logger.Errorf("error storing abr state: %v", err)
		return
	}

	err = s.dagStore.AddMany(ctx, abrWrapper.NewNodes)
	if err != nil {
		s.logger.Errorf("error storing abr new nodes: %v", err)
		return
	}
}

func (s *stateStorer) Receive(actorContext actor.Context) {
	switch msg := actorContext.Message().(type) {
	case *saveTransactionState:
		s.storeState(msg.ctx, msg.abrWrapper)
	default:
		s.logger.Debugf("state storage actor received unrecognized %T message: %+v", msg, msg)
	}
}
