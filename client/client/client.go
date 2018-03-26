package client

import (
	"github.com/quorumcontrol/qc3/notary"
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
	"github.com/google/uuid"
	"github.com/quorumcontrol/qc3/client/wallet"
	"github.com/gogo/protobuf/types"
	"reflect"
	"github.com/quorumcontrol/qc3/mailserver/mailserverpb"
	"github.com/ethereum/go-ethereum/rlp"
	"sync"
)

type Client struct {
	Group *notary.Group
	Wallet wallet.Wallet
	filter *whisper.Filter
	sessionKey *ecdsa.PrivateKey
	stopChan chan bool
	whisper *whisper.Whisper
	protocols map[string]chan *consensuspb.ProtocolResponse
}

func NewClient(group *notary.Group, walletImpl wallet.Wallet) *Client {
	sessionKey,_ := crypto.GenerateKey()
	return &Client{
		Group: group,
		Wallet: walletImpl,
		stopChan: make(chan bool),
		filter: network.NewP2PFilter(sessionKey),
		sessionKey: sessionKey,
		protocols: make(map[string]chan *consensuspb.ProtocolResponse),
	}
}

func (c *Client) Start() {
	ticker := time.NewTicker(1000 * time.Millisecond)
	log.Debug("starting client whisper", "key", c.sessionKey.PublicKey)
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
	log.Debug("CLIENT received message", "message", msg, "stats", c.whisper.Stats())
	resp := &consensuspb.ProtocolResponse{}
	err := proto.Unmarshal(msg.Payload, resp)
	if err != nil {
		log.Error("error unmarshaling message", "error", err)
		return
	}

	channel,ok := c.protocols[resp.Id]
	if !ok {
		log.Error("received response for unknown protocol", "id", resp.Id)
		return
	}

	channel<-resp
}


func (c *Client) Broadcast(symKey []byte, msg proto.Message) error {
	payload,err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("error marshaling any: %v", err)
	}

	return network.Send(c.whisper, &whisper.MessageParams{
		TTL: 60, // 1 minute TODO: what are the right TTL settings?
		KeySym: symKey,
		Topic: whisper.BytesToTopic(network.CothorityTopic),
		PoW: .02,  // TODO: what are the right settings for PoW?
		WorkTime: 10,
		Payload: payload,
	})
}

func (c *Client) RequestSignature(blocks []*consensuspb.Block, histories []*consensuspb.Chain) error {
	responseKey := crypto.FromECDSAPub(&c.sessionKey.PublicKey)

	signRequest := &consensuspb.SignatureRequest{
		Blocks: blocks,
		Histories: histories,
		ResponseKey: responseKey,
	}

	req := &consensuspb.ProtocolRequest{
		Id: uuid.New().String(),
		Request: objToAny(signRequest),
	}

	respChan := make(chan *consensuspb.ProtocolResponse)

	c.protocols[req.Id] = respChan

	err := c.Broadcast(crypto.Keccak256([]byte(c.Group.Id)), req)

	if err != nil {
		return fmt.Errorf("error sending: %v", err)
	}

	timeout := time.NewTimer(2000 * time.Millisecond)

	responses := make(map[string]*consensuspb.SignatureResponse)
	var respMutex sync.Mutex

	go func() {
		for {
			select {
			case protocolResponse := <- respChan:
				_signatureResponse,err := anyToObj(protocolResponse.Response)
				if err != nil {
					log.Error("error converting protocol response", "error", err)
				}
				resp := _signatureResponse.(*consensuspb.SignatureResponse)
				existing,err := c.Wallet.GetChain(resp.Block.SignableBlock.ChainId)
				if err != nil {
					log.Error("error getting chain", "error", err)
					return
				}
				if existing != nil {
					log.Info("appending signatures to block", "signatures", resp.Block.Signatures)
					existing.Blocks[len(existing.Blocks) - 1].Signatures = append(existing.Blocks[len(existing.Blocks) - 1].Signatures, resp.Block.Signatures...)
					c.Wallet.SetChain(existing.Id, existing)
				} else {
					log.Error("received block for unknown chain", "chainId", resp.Block.SignableBlock.ChainId)
				}

				respMutex.Lock()
				responses[resp.SignerId] = resp
				respMutex.Unlock()

				if len(responses) == len(c.Group.SortedPublicKeys) {
					log.Debug("All Responses In", "id", resp.SignerId)
					_,err = c.Group.ReplaceSignatures(existing.Blocks[len(existing.Blocks) - 1])
					if err != nil {
						log.Error("error replacing signatures", "error", err)
					}
					c.Wallet.SetChain(existing.Id, existing)
				}

			case <-timeout.C:

			}
		}
	}()

	return nil
}

func (c *Client) CreateChain(key *ecdsa.PrivateKey) (*consensuspb.Chain, error) {
	chain := consensus.ChainFromEcdsaKey(&key.PublicKey)
	chain.Blocks = append(chain.Blocks, &consensuspb.Block{
		SignableBlock: &consensuspb.SignableBlock{
			ChainId: chain.Id,
			Sequence: 0,
			Transactions: []*consensuspb.Transaction{
				{
					Id: uuid.New().String(),
					Type: consensuspb.ADD_DATA,
					Payload: []byte("genesis chain"),
				},
			},
		},
	})

	consensus.OwnerSignBlock(chain.Blocks[0], key)

	c.Wallet.SetChain(chain.Id, chain)

	err := c.RequestSignature(chain.Blocks, nil)
	if err != nil {
		return nil, fmt.Errorf("error requesting signature: %v", err)
	}

	return chain, nil
}

//func (c *Client) GetTip(chainId string) {
//	tipMsg := &consensuspb.TipRequest{
//		Id: chainId,
//	}
//}

func (c *Client) SendMessage(symKeyForAgent []byte, destKey *ecdsa.PublicKey, msg proto.Message) (error) {
	any := objToAny(msg)

	payload,err := proto.Marshal(any)
	if err != nil {
		return fmt.Errorf("error marshaling any: %v", err)
	}

	params := &whisper.MessageParams{
		TTL: 60,
		Dst: destKey,
		Topic: whisper.BytesToTopic(network.CothorityTopic),
		PoW: .02,  // TODO: what are the right settings for PoW?
		WorkTime: 10,
		Payload: payload,
	}
	innerMsg,err := whisper.NewSentMessage(params)
	env,err := innerMsg.Wrap(params)
	if err != nil {
		return fmt.Errorf("error wrapping: %v", err)
	}
	envBytes,err := rlp.EncodeToBytes(env)
	if err != nil {
		return fmt.Errorf("error encoding: %v", err)
	}

	nestedEnvelope := &mailserverpb.NestedEnvelope{
		Envelope: envBytes,
		Destination: crypto.FromECDSAPub(destKey),
	}

	req := &consensuspb.ProtocolRequest{
		Id: uuid.New().String(),
		Request: objToAny(nestedEnvelope),
	}

	return c.Broadcast(symKeyForAgent, req)
}

func anyToObj(any *types.Any) (proto.Message, error) {
	typeName := any.TypeUrl
	instanceType := proto.MessageType(typeName)
	log.Debug("unmarshaling from Any type to type: %v from typeName %s", "type", instanceType, "name", typeName)

	// instanceType will be a pointer type, so call Elem() to get the original Type and then interface
	// so that we can change it to the kind of object we want
	instance := reflect.New(instanceType.Elem()).Interface()
	err := proto.Unmarshal(any.GetValue(), instance.(proto.Message))
	if err != nil {
		return nil, err
	}
	return instance.(proto.Message), nil
}

func objToAny(obj proto.Message) (*types.Any) {
	objectType := proto.MessageName(obj)
	bytes, err := proto.Marshal(obj)
	if err != nil {
		log.Crit("error marshaling", "error", err)
		return nil
	}
	return &types.Any{
		TypeUrl: objectType,
		Value:   bytes,
	}
}