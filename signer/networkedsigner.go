package signer

import (
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/typecaster"
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

	handler.AssignHandler("ADD_BLOCK", ns.AddBlockHandler)

	return ns
}

func (ns *NetworkedSigner) AddBlockHandler(req network.Request) (*network.Response, error) {
	addBlockrequest := &AddBlockRequest{}
	err := typecaster.ToType(req.Payload, addBlockrequest)
	if err != nil {
		return nil, fmt.Errorf("error getting payload: %v", err)
	}

	resp, err := ns.Signer.ProcessRequest(addBlockrequest)
	if err != nil {
		return nil, fmt.Errorf("error signing: %v", err)
	}

	return &network.Response{
		Id:      req.Id,
		Payload: resp,
		Code:    200,
	}, nil
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
