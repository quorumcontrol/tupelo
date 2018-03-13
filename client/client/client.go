package client

import (
	"github.com/quorumcontrol/qc3/notary"
	"github.com/quorumcontrol/qc3/client/wallet"
	"crypto/ecdsa"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	whisper "github.com/ethereum/go-ethereum/whisper/whisperv5"
	"github.com/quorumcontrol/qc3/network"
	"github.com/ethereum/go-ethereum/crypto"
	"time"
	"github.com/ethereum/go-ethereum/log"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/gogo/protobuf/proto"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
)

type Client struct {
	Group *notary.Group
	Wallet *wallet.Wallet
	filter *whisper.Filter
	sessionKey *ecdsa.PrivateKey
	stopChan chan bool
	whisper *whisper.Whisper
	protocols map[string]map[string]*consensuspb.SignatureResponse
}

func NewClient(group *notary.Group, wallet *wallet.Wallet) *Client {
	sessionKey,_ := crypto.GenerateKey()
	return &Client{
		Group: group,
		Wallet: wallet,
		stopChan: make(chan bool),
		filter: network.NewP2PFilter(sessionKey),
		sessionKey: sessionKey,
		protocols: make(map[string]map[string]*consensuspb.SignatureResponse),
	}
}

func (c *Client) Start() {
	ticker := time.NewTicker(1000 * time.Millisecond)
	log.Debug("starting client whisper")
	whisp := network.Start(c.sessionKey)
	c.whisper = whisp
	whisp.Subscribe(c.filter)

	go func() {
		for {
			select {
			case <-ticker.C:
				msgs := c.filter.Retrieve()
				for _,msg := range msgs {
					c.handleMessage(msg)
				}
			case <-c.stopChan:
				log.Debug("stop chan")
				whisp.Stop()
				return
			}
		}
	}()
}

func (c *Client) Stop() {
	c.stopChan<-true
}

func (c *Client) handleMessage(msg *whisper.ReceivedMessage) {
	log.Debug("received message", "message", msg, "stats", c.whisper.Stats())
	resp := &consensuspb.SignatureResponse{}
	err := proto.Unmarshal(msg.Payload, resp)
	if err != nil {
		log.Error("error unmarshaling message", "error", err)
		return
	}

	c.protocols[resp.Id][resp.SignerId] = resp

	if len(c.protocols[resp.Id]) == len(c.Group.SortedPublicKeys) {
		log.Debug("All Responses In", "id", resp.Id)

	}

}


func (c *Client) broadcast(payload []byte) error {
	return network.Send(c.whisper, &whisper.MessageParams{
		TTL: 60, // 1 minute TODO: what are the right TTL settings?
		KeySym: common.StringToHash(c.Group.Id).Bytes(),
		Topic: whisper.BytesToTopic(network.CothorityTopic),
		PoW: .02,  // TODO: what are the right settings for PoW?
		WorkTime: 10,
		Payload: payload,
	})
}

func (c *Client) RequestSignature(chain *consensuspb.Chain) error {
	responseKey := crypto.FromECDSAPub(&c.sessionKey.PublicKey)

	signRequest := &consensuspb.SignatureRequest{
		Id: uuid.New().String(),
		Chain: chain,
		ResponseKey: responseKey,
	}

	c.protocols[signRequest.Id] = make(map[string]*consensuspb.SignatureResponse)
	requestBytes,_ := proto.Marshal(signRequest)

	err := c.broadcast(requestBytes)

	if err != nil {
		return fmt.Errorf("error sending: %v", err)
	}

	return nil
}

func (c *Client) CreateChain(key *ecdsa.PrivateKey) (*consensuspb.Chain, error) {
	chain := consensus.ChainFromEcdsaKey(&key.PublicKey)
	chain.Blocks = append(chain.Blocks, &consensuspb.Block{
		SignableBlock: &consensuspb.SignableBlock{
			ChainId: chain.Id,
			Transactions: []*consensuspb.Transaction{
				{
					Type: consensuspb.ADD_DATA,
					Payload: []byte("genesis chain"),
				},
			},
		},
	})

	consensus.OwnerSignBlock(chain.Blocks[0], key)

	err := c.RequestSignature(chain)
	if err != nil {
		return nil, fmt.Errorf("error requesting signature: %v", err)
	}

	return chain, nil
}
