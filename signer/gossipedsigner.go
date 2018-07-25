package signer

import (
	"context"

	"fmt"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/typecaster"
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
	handler.AssignHandler(consensus.MessageType_AddBlock, gossipSigner.AddBlockHandler)

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

func (gs *GossipedSigner) stateHandler(ctx context.Context, group *consensus.Group, objectId, transaction, currentState []byte) (nextState []byte, accepted bool, err error) {
	addBlockrequest := &consensus.AddBlockRequest{}
	err = cbornode.DecodeInto(transaction, addBlockrequest)
	if err != nil {
		return nil, false, fmt.Errorf("error getting payload: %v", err)
	}

	var storedTip *cid.Cid

	if len(currentState) > 1 {
		storedTip, err = cid.Cast(currentState)
		if err != nil {
			log.Error("error casting state into CID", "err", err)
			return nil, false, &consensus.ErrorCode{Memo: fmt.Sprintf("error casting: %v", err), Code: consensus.ErrUnknown}
		}
	}

	resp, err := gs.signer.ProcessAddBlock(storedTip, addBlockrequest)
	if err != nil {
		log.Error("error processing block", "err", err)
		return nil, false, nil
	}

	return resp.Tip.Bytes(), true, nil
}

func (gs *GossipedSigner) acceptedHandler(ctx context.Context, group *consensus.Group, _, transaction, newState []byte) (err error) {
	addBlockrequest := &consensus.AddBlockRequest{}
	err = cbornode.DecodeInto(transaction, addBlockrequest)
	if err != nil {
		return fmt.Errorf("error getting payload: %v", err)
	}

	if trans := stakeTransactionFromBlock(addBlockrequest.NewBlock); trans != nil {
		log.Debug("new stake")
		payload := &consensus.StakePayload{}
		err := typecaster.ToType(trans.Payload, payload)
		if err != nil {
			return fmt.Errorf("error casting payload: %v", err)
		}
		newMem := consensus.NewRemoteNode(payload.VerKey, payload.DstKey)
		log.Info("new stake", "member", newMem.Id)
		group.AddMember(newMem)
	}

	return nil
}

func (gs *GossipedSigner) AddBlockHandler(ctx context.Context, addBlockNetworkReq network.Request, respChan network.ResponseChan) error {
	addBlockrequest := &consensus.AddBlockRequest{}
	err := cbornode.DecodeInto(addBlockNetworkReq.Payload, addBlockrequest)
	if err != nil {
		return fmt.Errorf("error getting payload: %v", err)
	}

	log.Debug("add block handler", "tip", addBlockrequest.Tip, "request", addBlockrequest)

	gossipMessage := &gossip.GossipMessage{
		ObjectId:    []byte(addBlockrequest.ChainId),
		Transaction: addBlockNetworkReq.Payload,
	}

	gossipReq, err := network.BuildRequest(gossip.MessageType_Gossip, gossipMessage)
	if err != nil {
		return fmt.Errorf("error building gossip message")
	}

	internalRespChan := make(network.ResponseChan, 1)

	err = gs.gossiper.HandleGossipRequest(ctx, *gossipReq, internalRespChan)
	if err != nil {
		return fmt.Errorf("error handling request")
	}

	resp, err := network.BuildResponse(addBlockNetworkReq.Id, 200, <-internalRespChan)
	if err != nil {
		return fmt.Errorf("error building response: %v", err)
	}
	respChan <- resp

	return nil
}

func stakeTransactionFromBlock(block *chaintree.BlockWithHeaders) *chaintree.Transaction {
	for _, trans := range block.Block.Transactions {
		if trans.Type == consensus.TransactionTypeStake {
			return trans
		}
	}
	return nil
}
