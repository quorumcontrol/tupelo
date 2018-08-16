package signer

import (
	"fmt"

	"context"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/storage"
)

type NetworkedSigner struct {
	Node    *network.Node
	Server  *network.MessageHandler
	Signer  *Signer
	Storage storage.Storage
}

var DidBucket = []byte("tips")
var SigBucket = []byte("sigs")

func NewNetworkedSigner(node *network.Node, signer *Signer, store storage.Storage) *NetworkedSigner {
	handler := network.NewMessageHandler(node, []byte(signer.Group.Id()))

	ns := &NetworkedSigner{
		Node:    node,
		Server:  handler,
		Signer:  signer,
		Storage: store,
	}

	handler.AssignHandler(consensus.MessageType_AddBlock, ns.AddBlockHandler)
	handler.AssignHandler(consensus.MessageType_Feedback, ns.FeedbackHandler)
	handler.AssignHandler(consensus.MessageType_TipRequest, ns.TipHandler)

	ns.SetupStorage()

	return ns
}

func (ns *NetworkedSigner) SetupStorage() {
	ns.Storage.CreateBucketIfNotExists(DidBucket)
	ns.Storage.CreateBucketIfNotExists(SigBucket)
}

func (ns *NetworkedSigner) AddBlockHandler(_ context.Context, req network.Request, respChan network.ResponseChan) error {
	addBlockrequest := &consensus.AddBlockRequest{}
	err := cbornode.DecodeInto(req.Payload, addBlockrequest)
	if err != nil {
		return fmt.Errorf("error getting payload: %v", err)
	}

	log.Debug("add block handler", "tip", addBlockrequest.Tip, "request", addBlockrequest)

	storedBytes, err := ns.Storage.Get(DidBucket, []byte(addBlockrequest.ChainId))
	if err != nil {
		return &consensus.ErrorCode{Memo: fmt.Sprintf("error getting storage: %v", err), Code: consensus.ErrUnknown}
	}

	var storedTip *cid.Cid

	if len(storedBytes) > 0 {
		storedTip, err = cid.Cast(storedBytes)
		if err != nil {
			return &consensus.ErrorCode{Memo: fmt.Sprintf("error casting: %v", err), Code: consensus.ErrUnknown}
		}
	}

	addBlockResp, err := ns.Signer.ProcessAddBlock(storedTip, addBlockrequest)
	if err != nil {
		return fmt.Errorf("error signing: %v", err)
	}

	resp, err := network.BuildResponse(req.Id, 200, addBlockResp)
	if err != nil {
		return fmt.Errorf("error building response: %v", err)
	}
	ns.Storage.Set(DidBucket, []byte(addBlockrequest.ChainId), addBlockResp.Tip.Bytes())

	respChan <- resp

	return nil
}

func (ns *NetworkedSigner) FeedbackHandler(_ context.Context, req network.Request, respChan network.ResponseChan) error {
	feedbackRequest := &consensus.FeedbackRequest{}
	err := cbornode.DecodeInto(req.Payload, feedbackRequest)
	if err != nil {
		return fmt.Errorf("error getting payload: %v", err)
	}

	log.Debug("feedback handler", "tip", feedbackRequest.Tip, "request", feedbackRequest)

	verified, err := ns.Signer.Group.VerifySignature(consensus.MustObjToHash(feedbackRequest.Tip.Bytes()), &feedbackRequest.Signature)

	if err != nil {
		return fmt.Errorf("error verifying signature: %v", err)
	}

	feedbackResp, err := network.BuildResponse(req.Id, 200, true)
	if err != nil {
		return fmt.Errorf("error building response: %v", err)
	}

	if verified {
		sw := &safewrap.SafeWrap{}
		node := sw.WrapObject(feedbackRequest)
		if sw.Err != nil {
			return fmt.Errorf("error wrapping: %v", sw.Err)
		}

		log.Debug("setting signature", "tip", feedbackRequest.Tip)
		ns.Storage.Set(SigBucket, []byte(feedbackRequest.ChainId), node.RawData())
	} else {
		log.Debug("verified", "verified", verified)
		return fmt.Errorf("error, unverified")
	}

	respChan <- feedbackResp

	return nil
}

func (ns *NetworkedSigner) TipHandler(_ context.Context, req network.Request, respChan network.ResponseChan) error {
	tipRequest := &consensus.TipRequest{}
	err := cbornode.DecodeInto(req.Payload, tipRequest)
	if err != nil {
		return fmt.Errorf("error getting payload: %v", err)
	}

	log.Debug("received tip request", "req", req)

	feedbackBytes, err := ns.Storage.Get(SigBucket, []byte(tipRequest.ChainId))
	if len(feedbackBytes) == 0 || err != nil {
		return fmt.Errorf("error getting chain id: %v", err)
	}

	feedbackRequest := &consensus.FeedbackRequest{}
	err = cbornode.DecodeInto(feedbackBytes, feedbackRequest)
	if err != nil {
		return fmt.Errorf("error decoding: %v", err)
	}

	tipResponse := &consensus.TipResponse{
		ChainId:   feedbackRequest.ChainId,
		Tip:       feedbackRequest.Tip,
		Signature: feedbackRequest.Signature,
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
