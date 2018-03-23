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
	"github.com/ethereum/go-ethereum/common"
	"github.com/google/uuid"
	"github.com/quorumcontrol/qc3/client/wallet"
	"github.com/gogo/protobuf/types"
	"reflect"
)

type Client struct {
	Group *notary.Group
	Wallet wallet.Wallet
	filter *whisper.Filter
	sessionKey *ecdsa.PrivateKey
	stopChan chan bool
	whisper *whisper.Whisper
	protocols map[string]map[string]*consensuspb.SignatureResponse
}

func NewClient(group *notary.Group, walletImpl wallet.Wallet) *Client {
	sessionKey,_ := crypto.GenerateKey()
	return &Client{
		Group: group,
		Wallet: walletImpl,
		stopChan: make(chan bool),
		filter: network.NewP2PFilter(sessionKey),
		sessionKey: sessionKey,
		protocols: make(map[string]map[string]*consensuspb.SignatureResponse),
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
	respAny := &types.Any{}
	err := proto.Unmarshal(msg.Payload, respAny)
	if err != nil {
		log.Error("error unmarshaling message", "error", err)
		return
	}

	respProto,err := anyToObj(respAny)
	if err != nil {
		log.Error("error converting any to known message type", "type", respAny.TypeUrl)
		return
	}
	resp := respProto.(*consensuspb.SignatureResponse)

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

	c.protocols[resp.Id][resp.SignerId] = resp

	//TODO: handle timeouts
	if len(c.protocols[resp.Id]) == len(c.Group.SortedPublicKeys) {
		log.Debug("All Responses In", "id", resp.Id)
		_,err = c.Group.ReplaceSignatures(existing.Blocks[len(existing.Blocks) - 1])
		if err != nil {
			log.Error("error replacing signatures", "error", err)
		}
		c.Wallet.SetChain(existing.Id, existing)
	}
}


func (c *Client) broadcast(msg proto.Message) error {
	any,err := objToAny(msg)
	if err != nil {
		return fmt.Errorf("error converting to any: %v", err)
	}
	payload,err := proto.Marshal(any)
	if err != nil {
		return fmt.Errorf("error marshaling any: %v", err)
	}

	return network.Send(c.whisper, &whisper.MessageParams{
		TTL: 60, // 1 minute TODO: what are the right TTL settings?
		KeySym: common.StringToHash(c.Group.Id).Bytes(),
		Topic: whisper.BytesToTopic(network.CothorityTopic),
		PoW: .02,  // TODO: what are the right settings for PoW?
		WorkTime: 10,
		Payload: payload,
	})
}

func (c *Client) RequestSignature(blocks []*consensuspb.Block, histories []*consensuspb.Chain) error {
	responseKey := crypto.FromECDSAPub(&c.sessionKey.PublicKey)

	signRequest := &consensuspb.SignatureRequest{
		Id: uuid.New().String(),
		Blocks: blocks,
		Histories: histories,
		ResponseKey: responseKey,
	}

	c.protocols[signRequest.Id] = make(map[string]*consensuspb.SignatureResponse)

	err := c.broadcast(signRequest)

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

func objToAny(obj proto.Message) (*types.Any, error) {
	objectType := proto.MessageName(obj)
	bytes, err := proto.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return &types.Any{
		TypeUrl: objectType,
		Value:   bytes,
	}, nil
}