package signer

import (
	"context"

	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/gossip"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/storage"
)

type GossipedSigner struct {
	gossiper *gossip.Gossiper
	signer   *Signer
	started  bool
}

func GroupToTopic(group *consensus.Group) []byte {
	return []byte(group.Id())
}

func NewGossipedSigner(node *network.Node, signer *Signer, store storage.Storage) *GossipedSigner {

	gossipSigner := &GossipedSigner{
		signer: signer,
	}

	handler := network.NewMessageHandler(node, GroupToTopic(signer.Group))

	gossiper := gossip.NewGossiper(&gossip.GossiperOpts{
		MessageHandler:     handler,
		SignKey:            signer.SignKey,
		Group:              signer.Group,
		Storage:            store,
		StateHandler:       gossipSigner.stateHandler,
		AcceptedHandler:    gossipSigner.acceptedHandler,
		NumberOfGossips:    5,
		TimeBetweenGossips: 200,
	})

	gossipSigner.gossiper = gossiper

	return gossipSigner
}

func (gs *GossipedSigner) Start() {
	if gs.started {
		return
	}
	gs.started = true
	gs.gossiper.Start()
}

func (gs *GossipedSigner) Stop() {
	if !gs.started {
		return
	}
	gs.started = false
	gs.gossiper.Stop()
}

func (gs *GossipedSigner) stateHandler(ctx context.Context, currentState []byte, transaction []byte) (nextState []byte, err error) {
	return []byte{}, nil
}

func (gs *GossipedSigner) acceptedHandler(ctx context.Context, group *consensus.Group, newState []byte, transaction []byte) (err error) {
	return nil
}
