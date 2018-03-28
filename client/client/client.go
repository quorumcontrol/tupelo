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
	"github.com/quorumcontrol/qc3/mailserver/mailserverpb"
	"github.com/ethereum/go-ethereum/rlp"
	"sync"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/quorumcontrol/qc3/mailserver"
	"github.com/gogo/protobuf/types"
)

type Client struct {
	Group *notary.Group
	Wallet wallet.Wallet
	currentIdentity *ecdsa.PrivateKey
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

func (c *Client) SetCurrentIdentity(key *ecdsa.PrivateKey) {
	c.currentIdentity = key
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
		Src: c.sessionKey,
	})
}

func (c *Client) GetMessages(agent *ecdsa.PrivateKey) (chan proto.Message, error) {
	replayRequest := &mailserverpb.ReplayRequest{
		Destination: crypto.FromECDSAPub(&agent.PublicKey),
	}

	req := msgToProtocolRequest(replayRequest)

	respChan := make(chan *consensuspb.ProtocolResponse)

	c.protocols[req.Id] = respChan
	err := c.Broadcast(mailserver.AlphaMailServerKey, req)

	if err != nil {
		return nil, fmt.Errorf("error sending: %v", err)
	}

	retChan := make(chan proto.Message, len(c.Group.SortedPublicKeys) + 1)

	deDuper := &sync.Map{}

	go func() {
		timeout := time.NewTimer(10000 * time.Millisecond)
		for {
			select {
				case <-timeout.C:
					log.Debug("timeout received on get messages")
					return
				case resp := <- respChan:
					log.Debug("get messages received", "resp", resp)

					_,existed := deDuper.LoadOrStore(crypto.Keccak256Hash(resp.Response.Value).String(), true)

					if !existed {
						replayMsg,err := consensus.AnyToObj(resp.Response)
						if err != nil {
							log.Error("error converting response", "error", err)
							continue
						}
						replayResponse := replayMsg.(*mailserverpb.ReplayResponse)
						env,err := mailserver.EnvFromBytes(replayResponse.Envelope)
						if err != nil {
							log.Error("error getting envelope", "error", err)
							continue
						}

						msg,err := env.OpenAsymmetric(agent)
						if err != nil {
							log.Error("error opening envelope", "error", err)
							continue
						}
						isValidated := msg.Validate()
						log.Debug("message validation", "validated", isValidated)

						any := &types.Any{}
						err = proto.Unmarshal(msg.Payload, any)
						if err != nil {
							log.Error("error unmarshaling payload", "error", err)
							continue
						}

						ack := &mailserverpb.AckEnvelope{
							Destination:  crypto.FromECDSAPub(&agent.PublicKey),
							EnvelopeHash: env.Hash().Bytes(),
						}

						req := msgToProtocolRequest(ack)

						err = c.Broadcast(mailserver.AlphaMailServerKey, req)

						if err != nil {
							log.Error("error sending", "error",err)
							continue
						}

						retChan<-any
					}
			}
		}
	}()

	return retChan, nil
}

func (c *Client) processMailserverReceived(any *types.Any) {
	switch any.TypeUrl {
	case proto.MessageName(&consensuspb.SendCoinMessage{}):
		log.Debug("you got coin!")
	case proto.MessageName(&mailserverpb.ChatMessage{}):
		log.Debug("you got mail")
	default:
		log.Error("unknown message type received", "typeUrl", any.TypeUrl)
	}
}


// BUG: I think this has a bug when requesting multiple blocks to be signed
// where we're only counting responses and the signers could be sending one
// block at a time
// In fact, TODO: rewrite this whole method - the logic doesn't make sense as advertised
// it is confusing an array of blocks with a chain and doing no checking on whether
// the returned block is the last block in the saved chain or not
func (c *Client) RequestSignature(blocks []*consensuspb.Block, histories []*consensuspb.Chain) (chan bool, error) {
	responseKey := crypto.FromECDSAPub(&c.sessionKey.PublicKey)

	signRequest := &consensuspb.SignatureRequest{
		Blocks: blocks,
		Histories: histories,
		ResponseKey: responseKey,
	}

	req := msgToProtocolRequest(signRequest)

	respChan := make(chan *consensuspb.ProtocolResponse)

	c.protocols[req.Id] = respChan

	err := c.Broadcast(crypto.Keccak256([]byte(c.Group.Id)), req)

	if err != nil {
		return nil, fmt.Errorf("error sending: %v", err)
	}

	timeout := time.NewTimer(2000 * time.Millisecond)

	responses := make(map[string]*consensuspb.SignatureResponse)
	var respMutex sync.Mutex

	returnChan := make(chan bool)

	go func() {
		for {
			select {
			case protocolResponse := <- respChan:
				_signatureResponse,err := consensus.AnyToObj(protocolResponse.Response)
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
					log.Info("appending signatures to block", "signatures", resp.Block.Signatures, "transactionSignatures", resp.Block.TransactionSignatures)
					existing.Blocks[len(existing.Blocks) - 1].Signatures = append(existing.Blocks[len(existing.Blocks) - 1].Signatures, resp.Block.Signatures...)
					existing.Blocks[len(existing.Blocks) - 1].TransactionSignatures = append(existing.Blocks[len(existing.Blocks) - 1].TransactionSignatures, resp.Block.TransactionSignatures...)
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
					returnChan<-true
					return
				}

			case <-timeout.C:
				log.Error("timeout received")
			}
		}
	}()

	return returnChan, nil
}

func (c *Client) CreateChain(key *ecdsa.PrivateKey) (*consensuspb.Chain, error) {
	chain := consensus.ChainFromEcdsaKey(&key.PublicKey)
	chain.Blocks = append(chain.Blocks, &consensuspb.Block{
		SignableBlock: &consensuspb.SignableBlock{
			ChainId: chain.Id,
			Sequence: 0,
			Transactions: []*consensuspb.Transaction{
				consensus.EncapsulateTransaction(consensuspb.UPDATE_OWNERSHIP, &consensuspb.UpdateOwnershipTransaction{
					Agent: crypto.FromECDSAPub(&key.PublicKey),
					Authentication: &consensuspb.Authentication{
						PublicKeys: []*consensuspb.PublicKey{
							consensus.EcdsaToPublicKey(&key.PublicKey),
						},
					},
				}),
			},
		},
	})

	consensus.OwnerSignBlock(chain.Blocks[0], key)

	c.Wallet.SetChain(chain.Id, chain)

	respChan, err := c.RequestSignature(chain.Blocks, nil)
	if err != nil {
		return nil, fmt.Errorf("error requesting signature: %v", err)
	}
	//TODO: this is just for blocking, maybe make sync/async versions of all these methods
	<-respChan

	return chain, nil
}

func (c *Client) MintCoin(chainId, coinName string, amount int) (chan bool, error) {
	existing,err := c.Wallet.GetChain(chainId)
	if err != nil {
		log.Error("error getting chain", "error", err)
		return nil, fmt.Errorf("error getting existing chain: %v", err)
	}

	coinTransaction := consensus.EncapsulateTransaction(consensuspb.MINT_COIN, &consensuspb.MintCoinTransaction{
		Amount: uint64(amount),
		Name: existing.Id + ":" + coinName,
	})

	block,err := consensus.BlockWithTransactions(existing.Id, []*consensuspb.Transaction{coinTransaction}, existing.Blocks[len(existing.Blocks)-1])
	if err != nil {
		return nil, fmt.Errorf("error creating new blocK: %v", err)
	}

	signed,err := consensus.OwnerSignBlock(block, c.currentIdentity)
	if err != nil {
		return nil, fmt.Errorf("error creating current identity: %v", err)
	}

	existing.Blocks = append(existing.Blocks, signed)
	c.Wallet.SetChain(existing.Id, existing)

	respChan, err := c.RequestSignature([]*consensuspb.Block{signed}, []*consensuspb.Chain{existing})
	if err != nil {
		return nil, fmt.Errorf("error requesting signature: %v", err)
	}
	//TODO: this is just for blocking, maybe make sync/async versions of all these methods
	return respChan, nil
}

func (c *Client) SendCoin(chainId, destination, coinName string, amount int) (error) {
	destinationTipChan,err := c.GetTip(destination)
	if err != nil {
		return fmt.Errorf("error getting destination: %v", err)
	}

	destinationTip := <-destinationTipChan

	if destinationTip.Agent == nil {
		return fmt.Errorf("destination must have an agent to prevent money loss")
	}

	existing,err := c.Wallet.GetChain(chainId)
	if err != nil {
		log.Error("error getting chain", "error", err)
		return fmt.Errorf("error getting existing chain: %v", err)
	}

	sendTransaction := consensus.EncapsulateTransaction(consensuspb.SEND_COIN, &consensuspb.SendCoinTransaction{
		Amount: uint64(amount),
		Name: existing.Id + ":" + coinName,
		Destination: destination,
	})

	block,err := consensus.BlockWithTransactions(existing.Id, []*consensuspb.Transaction{sendTransaction}, existing.Blocks[len(existing.Blocks)-1])
	if err != nil {
		return fmt.Errorf("error creating new blocK: %v", err)
	}

	signed,err := consensus.OwnerSignBlock(block, c.currentIdentity)
	if err != nil {
		return fmt.Errorf("error creating current identity: %v", err)
	}

	existing.Blocks = append(existing.Blocks, signed)
	c.Wallet.SetChain(existing.Id, existing)

	respChan, err := c.RequestSignature([]*consensuspb.Block{signed}, []*consensuspb.Chain{existing})
	if err != nil {
		return fmt.Errorf("error requesting signature: %v", err)
	}
	<-respChan

	// now we send the send coin to our destination

	existing,err = c.Wallet.GetChain(chainId)
	if err != nil {
		log.Error("error getting chain", "error", err)
		return fmt.Errorf("error getting existing chain: %v", err)
	}

	log.Debug("existing after sending coin", "existing", existing)

	// get the transaction signature
	sigs := consensus.TransactionSignaturesByMemo(existing.Blocks[len(existing.Blocks)-1])
	sig := sigs[hexutil.Encode([]byte("tx:" + sendTransaction.Id))]

	msg := &consensuspb.SendCoinMessage{
		Transaction: sendTransaction,
		Signature: sig[0],
		Memo: []byte("you got some alpha coins"),
	}

	err = c.SendMessage(mailserver.AlphaMailServerKey, crypto.ToECDSAPub(destinationTip.Agent), msg)
	if err != nil {
		return fmt.Errorf("error sending message")
	}
	// send message to the agent


	return nil
}

func (c *Client) GetTip(chainId string) (chan *consensuspb.ChainTip, error) {
	tipMsg := &consensuspb.TipRequest{
		ChainId: chainId,
	}
	req := msgToProtocolRequest(tipMsg)

	respChan := make(chan *consensuspb.ProtocolResponse)

	c.protocols[req.Id] = respChan

	err := c.Broadcast(crypto.Keccak256([]byte(c.Group.Id)), req)

	if err != nil {
		return nil, fmt.Errorf("error sending: %v", err)
	}

	timeout := time.NewTimer(2000 * time.Millisecond)

	responses := make([]*consensuspb.ChainTip, 0)
	var respMutex sync.Mutex
	chainTipChan := make(chan *consensuspb.ChainTip)

	go func() {
		for {
			select {
			case protocolResponse := <- respChan:
				_chainTip,err := consensus.AnyToObj(protocolResponse.Response)
				if err != nil {
					log.Error("error converting protocol response", "error", err)
				}
				chainTip := _chainTip.(*consensuspb.ChainTip)
				log.Debug("received chain tip response", "chainTip", chainTip)


				respMutex.Lock()
				responses = append(responses, chainTip)
				respMutex.Unlock()

				//TODO: handle 1/3 giving bad info
				if len(responses) >= 2 {
					areEqual := responses[len(responses)-1].Equal(responses[len(responses)-2])
					if !areEqual {
						log.Error("non-matching responses", "new", responses[len(responses)-1], "last", responses[len(responses)-2])
						close(chainTipChan)
						return
					}
					log.Debug("chain tips equal?", "equal", areEqual)
				}

				if len(responses) == len(c.Group.SortedPublicKeys) {
					log.Debug("All Responses In", "id", chainTip.Id)
					chainTipChan<- chainTip
					close(chainTipChan)
					return
				} else {
					log.Debug("response count", "count", len(responses))
				}
			case <-timeout.C:
				log.Error("timeout received")
				close(chainTipChan)
				return
			}
		}
	}()
	return chainTipChan, nil
}

func (c *Client) SendMessage(symKeyForAgent []byte, destKey *ecdsa.PublicKey, msg proto.Message) (error) {
	any := consensus.ObjToAny(msg)

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

	req := msgToProtocolRequest(nestedEnvelope)

	return c.Broadcast(symKeyForAgent, req)
}

func msgToProtocolRequest(msg proto.Message) *consensuspb.ProtocolRequest {
	return &consensuspb.ProtocolRequest{
		Id: uuid.New().String(),
		Request: consensus.ObjToAny(msg),
	}
}
