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
	Server *network.MessageHandler
	Signer *Signer
}

func NewNetworkedSigner(node *network.Node, signer *Signer) *NetworkedSigner {
	handler := network.NewMessageHandler(node, []byte(signer.Group.Id()))

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

func (ns *NetworkedSigner) AddBlockHandler(req network.Request, respChan network.ResponseChan) error {
	addBlockrequest := &consensus.AddBlockRequest{}
	err := cbornode.DecodeInto(req.Payload, addBlockrequest)
	if err != nil {
		return fmt.Errorf("error getting payload: %v", err)
	}

	log.Debug("add block handler", "tip", addBlockrequest.Tip, "request", addBlockrequest)

	addBlockResp, err := ns.Signer.ProcessAddBlock(addBlockrequest)
	if err != nil {
		return fmt.Errorf("error signing: %v", err)
	}

	resp, err := network.BuildResponse(req.Id, 200, addBlockResp)
	if err != nil {
		return fmt.Errorf("error building response: %v", err)
	}

	respChan <- resp

	return nil
}

func (ns *NetworkedSigner) FeedbackHandler(req network.Request, respChan network.ResponseChan) error {
	feedbackRequest := &consensus.FeedbackRequest{}
	err := cbornode.DecodeInto(req.Payload, feedbackRequest)
	if err != nil {
		return fmt.Errorf("error getting payload: %v", err)
	}

	log.Debug("feedback handler", "tip", feedbackRequest.Tip, "request", feedbackRequest)

	err = ns.Signer.ProcessFeedback(feedbackRequest)
	if err != nil {
		return fmt.Errorf("error processing: %v", err)
	}

	resp, err := network.BuildResponse(req.Id, 200, true)
	if err != nil {
		return fmt.Errorf("error building response: %v", err)
	}

	respChan <- resp

	return nil
}

func (ns *NetworkedSigner) TipHandler(req network.Request, respChan network.ResponseChan) error {
	tipRequest := &consensus.TipRequest{}
	err := cbornode.DecodeInto(req.Payload, tipRequest)
	if err != nil {
		return fmt.Errorf("error getting payload: %v", err)
	}

	log.Debug("feedback handler", "chainid", tipRequest.ChainId, "request", tipRequest)

	tipResponse, err := ns.Signer.ProcessTipRequest(tipRequest)
	if err != nil {
		return fmt.Errorf("error processing: %v", err)
	}

	resp, err := network.BuildResponse(req.Id, 200, tipResponse)
	if err != nil {
		return fmt.Errorf("error building response: %v", err)
	}

	respChan <- resp

	return nil
}

func (ns *NetworkedSigner) Start() {
	ns.Node.Start()
	ns.Server.Start()
	ns.Server.HandleTopic([]byte(ns.Signer.Group.Id()), crypto.Keccak256([]byte(ns.Signer.Group.Id())))
}

func (ns *NetworkedSigner) Stop() {
	ns.Server.Stop()
	ns.Node.Stop()
}
