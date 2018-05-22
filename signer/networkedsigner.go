package signer

import (
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/network"
)

type NetworkedSigner struct {
	Node   *network.Node
	Server *network.RequestHandler
	Signer *Signer
}

func NewNetworkedSigner(node *network.Node, signer *Signer) *NetworkedSigner {
	handler := network.NewRequestHandler(node)

	ns := &NetworkedSigner{
		Node:   node,
		Server: handler,
		Signer: signer,
	}

	handler.AssignHandler(consensus.MessageType_AddBlock, ns.AddBlockHandler)
	handler.AssignHandler(consensus.MessageType_Feedback, ns.FeedbackHandler)
	handler.AssignHandler(consensus.MessageType_TipRequest, ns.TipHandler)

	return ns
}

func (ns *NetworkedSigner) AddBlockHandler(req network.Request) (*network.Response, error) {
	addBlockrequest := &consensus.AddBlockRequest{}
	err := cbornode.DecodeInto(req.Payload, addBlockrequest)
	if err != nil {
		return nil, fmt.Errorf("error getting payload: %v", err)
	}

	log.Debug("add block handler", "tip", addBlockrequest.Tip, "request", addBlockrequest)

	resp, err := ns.Signer.ProcessAddBlock(addBlockrequest)
	if err != nil {
		return nil, fmt.Errorf("error signing: %v", err)
	}

	return network.BuildResponse(req.Id, 200, resp)
}

func (ns *NetworkedSigner) FeedbackHandler(req network.Request) (*network.Response, error) {
	feedbackRequest := &consensus.FeedbackRequest{}
	err := cbornode.DecodeInto(req.Payload, feedbackRequest)
	if err != nil {
		return nil, fmt.Errorf("error getting payload: %v", err)
	}

	log.Debug("feedback handler", "tip", feedbackRequest.Tip, "request", feedbackRequest)

	err = ns.Signer.ProcessFeedback(feedbackRequest)
	if err != nil {
		return nil, fmt.Errorf("error processing: %v", err)
	}

	return network.BuildResponse(req.Id, 200, true)
}

func (ns *NetworkedSigner) TipHandler(req network.Request) (*network.Response, error) {
	tipRequest := &consensus.TipRequest{}
	err := cbornode.DecodeInto(req.Payload, tipRequest)
	if err != nil {
		return nil, fmt.Errorf("error getting payload: %v", err)
	}

	log.Debug("feedback handler", "chainid", tipRequest.ChainId, "request", tipRequest)

	tipResponse, err := ns.Signer.ProcessTipRequest(tipRequest)
	if err != nil {
		return nil, fmt.Errorf("error processing: %v", err)
	}

	return network.BuildResponse(req.Id, 200, tipResponse)
}

func (ns *NetworkedSigner) Start() {
	ns.Node.Start()
	ns.Server.Start()
	ns.Server.HandleTopic([]byte(ns.Signer.Group.Id), crypto.Keccak256([]byte(ns.Signer.Group.Id)))
}

func (ns *NetworkedSigner) Stop() {
	ns.Server.Stop()
	ns.Node.Stop()
}
