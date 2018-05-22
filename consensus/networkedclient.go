package consensus

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/qc3/network"
)

const MessageType_AddBlock = "ADD_BLOCK"
const MessageType_Feedback = "FEEDBACK"

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

//func (nc *NetworkedClient) SetSigningKey(key *ecdsa.PrivateKey) {
//	nc.signingKey = key
//}
