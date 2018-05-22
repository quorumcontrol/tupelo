package consensus

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/qc3/network"
)

const MessageType_AddBlock = "ADD_BLOCK"
const MessageType_Feedback = "FEEDBACK"
const MessageType_TipRequest = "TIP_REQUEST"

type NetworkedClient struct {
	sessionKey *ecdsa.PrivateKey
	Client     *network.Client
	Group      *Group
	Wallet     Wallet
	//signingKey *ecdsa.PrivateKey
}

func NewNetworkedClient(group *Group) (*NetworkedClient, error) {
	sessionKey, err := crypto.GenerateKey()
	if err != nil {
		return nil, fmt.Errorf("error generating session key: %v", err)
	}
	return &NetworkedClient{
		sessionKey: sessionKey,
		Client:     network.NewClient(sessionKey, []byte(group.Id), crypto.Keccak256([]byte(group.Id))),
		Group:      group,
	}, nil
}

func (nc *NetworkedClient) Stop() {
	nc.Client.Stop()
	nc.Client = network.NewClient(nc.sessionKey, []byte(nc.Group.Id), crypto.Keccak256([]byte(nc.Group.Id)))
}

func (nc *NetworkedClient) Start() {
	nc.Client.Start()
}

// TODO: this sends all nodes, only need to send certain nodes
// TODO: this stops being BFT at the moment if the last node returns a bad tip
// This function needs to be rewritten to actually be usable in production
func (nc *NetworkedClient) AddBlock(tree *SignedChainTree, block *chaintree.BlockWithHeaders) (*AddBlockResponse, error) {
	nodes := make([][]byte, len(tree.ChainTree.Dag.Nodes()))
	for i, node := range tree.ChainTree.Dag.Nodes() {
		nodes[i] = node.Node.RawData()
	}

	addBlockRequest := &AddBlockRequest{
		Nodes:    nodes,
		NewBlock: block,
		Tip:      tree.Tip(),
	}

	log.Debug("sending: ", "tip", addBlockRequest.Tip, "len(nodes)", len(nodes))

	req, err := network.BuildRequest(MessageType_AddBlock, addBlockRequest)
	if err != nil {
		return nil, fmt.Errorf("error building request: %v", err)
	}

	respChan, err := nc.Client.DoRequest(req)
	if err != nil {
		return nil, fmt.Errorf("error doing request: %v", err)
	}

	sigMap := make(SignatureMap)
	var sig *Signature
	var tip *cid.Cid

	isValid := func() (bool, error) {
		if len(sigMap) == 0 {
			return false, nil
		}

		sig, err = nc.Group.CombineSignatures(sigMap)
		if err != nil {
			return false, fmt.Errorf("error combining: %v", err)
		}
		verified, err := nc.Group.VerifySignature(MustObjToHash(tip.Bytes()), sig)
		if err != nil {
			return false, fmt.Errorf("error verifying: %v", err)
		}
		return verified, nil
	}

	for valid, err := isValid(); err == nil && !valid; valid, err = isValid() {
		networkResp := <-respChan
		log.Debug("network resp", "resp", networkResp)
		resp := &AddBlockResponse{}
		err = cbornode.DecodeInto(networkResp.Payload, resp)
		if err != nil {
			return nil, fmt.Errorf("error decoding request: %v", err)
		}
		log.Debug("add block response", "addBlockResponse", resp)
		tip = resp.Tip
		sigMap[resp.SignerId] = resp.Signature
	}

	if err != nil {
		return nil, fmt.Errorf("error doing network: %v", err)
	}

	treeId, _ := tree.Id()

	return &AddBlockResponse{
		Tip:       tip,
		Signature: *sig,
		ChainId:   treeId,
	}, nil
}

//func (nc *NetworkedClient) SetSigningKey(key *ecdsa.PrivateKey) {
//	nc.signingKey = key
//}
