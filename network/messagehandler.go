package network

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/google/uuid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/dag"
)

const DefaultTTL = 60

func init() {
	cbornode.RegisterCborType(Request{})
	cbornode.RegisterCborType(Response{})
	cbornode.RegisterCborType(Message{})
}

type ResponseChan chan *Response

type Message struct {
	Request  *Request
	Response *Response
	source   *ecdsa.PublicKey
}

type Request struct {
	Type        string
	Id          string
	Payload     []byte
	destination *ecdsa.PublicKey
	source      *ecdsa.PublicKey
}

type Response struct {
	Id      string
	Code    int
	Payload []byte
	source  *ecdsa.PublicKey
}

type HandlerFunc func(ctx context.Context, req Request, respChan ResponseChan) error

type MessageHandler struct {
	mainTopic           []byte
	node                *Node
	outstandingRequests map[string]chan *Response
	mappings            map[string]HandlerFunc
	subs                []*Subscription
	subscriptionLock    *sync.RWMutex
	requestLock         *sync.RWMutex
	closeChan           chan bool
	messageChan         chan *Message
	started             bool
}

func NewMessageHandler(node *Node, topic []byte) *MessageHandler {
	return &MessageHandler{
		mainTopic:           topic,
		node:                node,
		mappings:            make(map[string]HandlerFunc),
		outstandingRequests: make(map[string]chan *Response),
		closeChan:           make(chan bool, 2),
		messageChan:         make(chan *Message, 10),
		subscriptionLock:    &sync.RWMutex{},
		requestLock:         &sync.RWMutex{},
	}
}

func (rh *MessageHandler) AssignHandler(requestType string, handlerFunc HandlerFunc) error {
	rh.subscriptionLock.Lock()
	defer rh.subscriptionLock.Unlock()

	rh.mappings[requestType] = handlerFunc
	return nil
}

func (rh *MessageHandler) HandleTopic(topic []byte, symkey []byte) {
	rh.subscriptionLock.Lock()
	defer rh.subscriptionLock.Unlock()

	rh.subs = append(rh.subs, rh.node.SubscribeToTopic(topic, symkey))
}

func (rh *MessageHandler) HandleKey(topic []byte, key *ecdsa.PrivateKey) {
	log.Debug("HandleKey", "pubKey", crypto.PubkeyToAddress(key.PublicKey).String())
	rh.subscriptionLock.Lock()
	defer rh.subscriptionLock.Unlock()

	rh.subs = append(rh.subs, rh.node.SubscribeToKey(key))
}

func BuildResponse(id string, code int, payload interface{}) (*Response, error) {
	sw := &dag.SafeWrap{}
	node := sw.WrapObject(payload)
	if sw.Err != nil {
		return nil, fmt.Errorf("error wrapping: %v", sw.Err)
	}

	return &Response{
		Payload: node.RawData(),
		Id:      id,
		Code:    code,
	}, nil
}

func BuildRequest(reqType string, payload interface{}) (*Request, error) {
	sw := &dag.SafeWrap{}
	node := sw.WrapObject(payload)
	if sw.Err != nil {
		return nil, fmt.Errorf("error wrapping: %v", sw.Err)
	}

	return &Request{
		Payload: node.RawData(),
		Id:      uuid.New().String(),
		Type:    reqType,
	}, nil
}

func (rh *MessageHandler) DoRequest(destination *ecdsa.PublicKey, req *Request) (chan *Response, error) {
	log.Debug("do request")
	respChan := make(chan *Response)
	rh.requestLock.Lock()
	rh.outstandingRequests[req.Id] = respChan
	rh.requestLock.Unlock()

	wireBytes, err := requestToWireFormat(req)
	if err != nil {
		return nil, fmt.Errorf("error converting request: %v", err)
	}

	log.Debug("sending destination message", "id", req.Id, "destination", crypto.PubkeyToAddress(*destination).String(), "source", crypto.PubkeyToAddress(rh.node.key.PublicKey).String())
	err = rh.send(destination, wireBytes)
	if err != nil {
		return nil, fmt.Errorf("error sending: %v", err)
	}

	return respChan, nil
}

// Push sends a message to a destination, but does not wait for any response
func (rh *MessageHandler) Push(destination *ecdsa.PublicKey, req *Request) error {
	wireBytes, err := requestToWireFormat(req)
	if err != nil {
		return fmt.Errorf("error converting request: %v", err)
	}

	log.Debug("sending destination message", "id", req.Id, "destination", crypto.PubkeyToAddress(*destination).String(), "source", crypto.PubkeyToAddress(rh.node.key.PublicKey).String())
	err = rh.send(destination, wireBytes)
	if err != nil {
		return fmt.Errorf("error sending: %v", err)
	}

	return nil
}

// Broadcast will be deprecated when gossip, but leaving for current implementation
func (rh *MessageHandler) Broadcast(topic, symKey []byte, req *Request) (chan *Response, error) {
	respChan := make(chan *Response)
	rh.requestLock.Lock()
	rh.outstandingRequests[req.Id] = respChan
	rh.requestLock.Unlock()

	sw := &dag.SafeWrap{}
	reqNode := sw.WrapObject(&Message{Request: req})
	if sw.Err != nil {
		log.Error("error wrapping request", "err", sw.Err)
		return nil, fmt.Errorf("error wrapping request: %v", sw.Err)
	}

	log.Debug("sending message", "id", req.Id, "source", crypto.PubkeyToAddress(rh.node.key.PublicKey).String())
	err := rh.node.Send(MessageParams{
		Payload:  reqNode.RawData(),
		TTL:      DefaultTTL,
		PoW:      0.02,
		WorkTime: 10,
		KeySym:   symKey,
		Topic:    topic,
		Source:   rh.node.key,
	})

	if err != nil {
		return nil, fmt.Errorf("error sending: %v", err)
	}

	return respChan, nil
}

func (rh *MessageHandler) send(dst *ecdsa.PublicKey, payload []byte) error {
	return rh.node.Send(MessageParams{
		Payload:     payload,
		TTL:         DefaultTTL,
		PoW:         0.02,
		WorkTime:    10,
		Destination: dst,
		Source:      rh.node.key,
	})
}

func (rh *MessageHandler) handleRequest(req *Request) {
	log.Debug("request received", "type", req.Type)
	handler, ok := rh.mappings[req.Type]
	if ok {
		log.Debug("handling message", "id", req.Id)
		respChan := make(ResponseChan, 1)
		defer close(respChan)

		var resp *Response

		err := handler(context.Background(), *req, respChan)
		if err == nil {
			resp = <-respChan
		} else {
			log.Error("error handling message", "err", err)
			resp = &Response{
				Code:    500,
				Payload: []byte(err.Error()),
			}
		}

		if resp != nil {
			resp.Id = req.Id
			wireBytes, err := responseToWireFormat(resp)
			if err != nil {
				log.Error("error converting response", "err", err)
				return
			}
			log.Debug("responding", "id", req.Id, "destination", crypto.PubkeyToAddress(*req.source).String())

			err = rh.send(req.source, wireBytes)
			if err != nil {
				log.Error("error sending request", "err", err)
			}
		}
	} else {
		log.Info("invalid message type", "type", req.Type)
	}
}

func (rh *MessageHandler) handleResponse(resp *Response) {
	log.Debug("response received", "source", crypto.PubkeyToAddress(*resp.source).String())

	log.Debug("client sending response to response channel", "id", resp.Id)
	rh.requestLock.RLock()
	outChan, ok := rh.outstandingRequests[resp.Id]
	rh.requestLock.RUnlock()

	if ok {
		outChan <- resp
		close(outChan)
		rh.requestLock.Lock()
		delete(rh.outstandingRequests, resp.Id)
		rh.requestLock.Unlock()
	} else {
		log.Error("client received response for unknown message", "id", resp.Id)
	}
}

func (rh *MessageHandler) Start() {
	if rh.started {
		return
	}
	rh.started = true

	rh.node.Start()
	rh.HandleKey(rh.mainTopic, rh.node.key)

	go func() {
		for {
			select {
			case msg := <-rh.messageChan:
				log.Debug("message received", "msg", msg)
				if msg == nil {
					log.Error("received nil msg")
					continue
				}
				if msg.Request != nil {
					msg.Request.source = msg.source
					go rh.handleRequest(msg.Request)
					continue
				}
				if msg.Response != nil {
					msg.Response.source = msg.source
					go rh.handleResponse(msg.Response)
					continue
				}
				log.Error("message handler reached a loop it should not have")

			case <-rh.closeChan:
				return
			}
		}
	}()

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)

		for {
			select {
			case <-ticker.C:
				for _, sub := range rh.subs {
					messages := sub.RetrieveMessages()
					for _, msg := range messages {
						log.Debug("message received", "msg", msg)
						rh.messageChan <- messageToWireFormat(msg)
					}
				}
			case <-rh.closeChan:
				return
			}
		}
	}()
}

func (rh *MessageHandler) Stop() {
	if !rh.started {
		return
	}
	rh.node.Stop()
	rh.closeChan <- true
	rh.closeChan <- true
	rh.started = false
}

func messageToWireFormat(message *ReceivedMessage) *Message {
	msg := &Message{}

	err := cbornode.DecodeInto(message.Payload, msg)
	if err != nil {
		log.Error("invalid message", "err", err)
		return nil
	}
	msg.source = message.Source
	return msg
}

func responseToWireFormat(resp *Response) ([]byte, error) {
	sw := &dag.SafeWrap{}

	msg := &Message{
		Response: resp,
	}

	msgBytes := sw.WrapObject(msg)
	if sw.Err != nil {
		return nil, fmt.Errorf("error wrapping request: %v", sw.Err)
	}

	return msgBytes.RawData(), nil
}

func requestToWireFormat(req *Request) ([]byte, error) {
	sw := &dag.SafeWrap{}

	msg := &Message{
		Request: req,
	}

	msgBytes := sw.WrapObject(msg)
	if sw.Err != nil {
		return nil, fmt.Errorf("error wrapping request: %v", sw.Err)
	}

	return msgBytes.RawData(), nil
}
