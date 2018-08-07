package signer

import (
	"context"
	"time"

	"fmt"

	"sync"

	"github.com/ethereum/go-ethereum/crypto"
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

type pendingResponse struct {
	ch network.ResponseChan
	id string
}

type responseHolder map[string]*pendingResponse

type GossipedSigner struct {
	gossiper  *gossip.Gossiper
	signer    *Signer
	started   bool
	responses responseHolder
	respLock  *sync.RWMutex
}

func GroupToTopic(group *consensus.Group) []byte {
	return []byte(group.Id())
}

func NewGossipedSigner(node *network.Node, signer *Signer, store storage.Storage) *GossipedSigner {

	gossipSigner := &GossipedSigner{
		signer:    signer,
		responses: make(responseHolder),
		respLock:  &sync.RWMutex{},
	}

	handler := network.NewMessageHandler(node, GroupToTopic(signer.Group))

	gossiper := &gossip.Gossiper{
		MessageHandler:  handler,
		SignKey:         signer.SignKey,
		Group:           signer.Group,
		Storage:         store,
		StateHandler:    gossipSigner.stateHandler,
		AcceptedHandler: gossipSigner.acceptedHandler,
		Fanout:          5,
	}
	gossiper.Initialize()

	gossipSigner.gossiper = gossiper
	handler.AssignHandler(consensus.MessageType_AddBlock, gossipSigner.AddBlockHandler)
	handler.AssignHandler(consensus.MessageType_TipRequest, gossipSigner.TipHandler)

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

func (gs *GossipedSigner) rejectedHandler(ctx context.Context, group *consensus.Group, objectId, transaction, currentState []byte) (err error) {
	log.Debug("rejected handler")

	err = gs.respondToTransaction(objectId, transaction)
	if err != nil {
		return fmt.Errorf("error responding to trans: %v", err)
	}

	return nil
}

func (gs *GossipedSigner) acceptedHandler(ctx context.Context, group *consensus.Group, objectId, transaction, newState []byte) (err error) {
	log.Debug("accepted handler")
	addBlockrequest := &consensus.AddBlockRequest{}
	err = cbornode.DecodeInto(transaction, addBlockrequest)
	if err != nil {
		return fmt.Errorf("error getting payload: %v", err)
	}

	err = gs.respondToTransaction(objectId, transaction)
	if err != nil {
		return fmt.Errorf("error responding to trans: %v", err)
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
		ObjectID:    []byte(addBlockrequest.ChainId),
		Transaction: addBlockNetworkReq.Payload,
		PreviousTip: []byte(addBlockrequest.NewBlock.PreviousTip),
		Phase:       0,
		Round:       gs.gossiper.RoundAt(time.Now()),
	}

	pending := &pendingResponse{
		id: addBlockNetworkReq.Id,
		ch: respChan,
	}

	gs.respLock.Lock()
	gs.responses[string(crypto.Keccak256(addBlockNetworkReq.Payload))] = pending
	gs.respLock.Unlock()

	err = gs.gossiper.HandleGossip(ctx, gossipMessage)
	if err != nil {
		return fmt.Errorf("error handling request")
	}

	return nil
}

func (gs *GossipedSigner) TipHandler(_ context.Context, req network.Request, respChan network.ResponseChan) error {
	tipRequest := &consensus.TipRequest{}
	err := cbornode.DecodeInto(req.Payload, tipRequest)
	if err != nil {
		return fmt.Errorf("error getting payload: %v", err)
	}

	tipResponse, err := gs.tipForObject([]byte(tipRequest.ChainId))
	if err != nil {
		return fmt.Errorf("error getting tip: %v", err)
	}

	resp, err := network.BuildResponse(req.Id, 200, tipResponse)
	if err != nil {
		return fmt.Errorf("error building response: %v", err)
	}

	respChan <- resp

	return nil
}

func (gs *GossipedSigner) respondToTransaction(objectId, transaction []byte) error {
	log.Debug("respondToTransaction")
	transKey := string(crypto.Keccak256(transaction))

	gs.respLock.RLock()
	pending, ok := gs.responses[transKey]
	if ok {
		log.Debug("pending found, responding")
		gs.respLock.RUnlock()
		tipResponse, err := gs.tipForObject(objectId)
		if err != nil {
			return fmt.Errorf("error getting tip: %v", err)
		}

		resp, err := network.BuildResponse(pending.id, 200, tipResponse)
		if err != nil {
			return fmt.Errorf("error building response: %v", err)
		}

		pending.ch <- resp

		gs.respLock.Lock()
		defer gs.respLock.Unlock()
		delete(gs.responses, transKey)
	} else {
		gs.respLock.RUnlock()
	}
	return nil
}

func (gs *GossipedSigner) tipForObject(objectId []byte) (*consensus.TipResponse, error) {
	currState, err := gs.gossiper.GetCurrentState(objectId)
	if err != nil {
		return nil, fmt.Errorf("error getting state: %v", err)
	}

	var tip *cid.Cid

	if len(currState.Tip) > 0 {
		tip, err = cid.Cast(currState.Tip)
		if err != nil {
			return nil, fmt.Errorf("error casting tip: %v", err)
		}
	}

	return &consensus.TipResponse{
		ChainId:   string(objectId),
		Tip:       tip,
		Signature: currState.Signature,
	}, nil
}

func stakeTransactionFromBlock(block *chaintree.BlockWithHeaders) *chaintree.Transaction {
	for _, trans := range block.Block.Transactions {
		if trans.Type == consensus.TransactionTypeStake {
			return trans
		}
	}
	return nil
}
