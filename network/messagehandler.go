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
	"github.com/quorumcontrol/chaintree/safewrap"
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

func (r *Request) Source() *ecdsa.PublicKey {
	return r.source
}

type Response struct {
	Id      string
	Code    int
	Payload []byte
	source  *ecdsa.PublicKey
}

type HandlerFunc func(ctx context.Context, req Request, respChan ResponseChan) error

// MessageHandler is used to listen to and respond to events as well
// as send requests on the network.
type MessageHandler struct {
	Concurrency         int
	mainTopic           []byte
	node                *Node
	outstandingRequests map[string]chan *Response
	mappings            map[string]HandlerFunc
	subscriptionLock    *sync.RWMutex
	requestLock         *sync.RWMutex
	closeChan           chan bool
	messageChan         chan *Message
	started             bool
	requestChannel      chan *Request
	responseChannel     chan *Response
}

const defaultConcurrency = 10

func NewMessageHandler(node *Node, topic []byte) *MessageHandler {
	return &MessageHandler{
		Concurrency:         defaultConcurrency,
		mainTopic:           topic,
		node:                node,
		mappings:            make(map[string]HandlerFunc),
		outstandingRequests: make(map[string]chan *Response),
		closeChan:           make(chan bool, 2),
		messageChan:         make(chan *Message, 10),
		subscriptionLock:    &sync.RWMutex{},
		requestLock:         &sync.RWMutex{},
		requestChannel:      make(chan *Request),
		responseChannel:     make(chan *Response),
	}
}

func (rh *MessageHandler) AssignHandler(requestType string, handlerFunc HandlerFunc) error {
	rh.subscriptionLock.Lock()
	defer rh.subscriptionLock.Unlock()

	rh.mappings[requestType] = handlerFunc
	return nil
}

func BuildResponse(id string, code int, payload interface{}) (*Response, error) {
	sw := &safewrap.SafeWrap{}
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
	sw := &safewrap.SafeWrap{}
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

	log.Trace("sending destination message", "id", req.Id, "destination", crypto.PubkeyToAddress(*destination).String(), "source", crypto.PubkeyToAddress(rh.node.key.PublicKey).String())
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

	log.Trace("sending destination message", "id", req.Id, "destination", crypto.PubkeyToAddress(*destination).String(), "source", crypto.PubkeyToAddress(rh.node.key.PublicKey).String())
	err = rh.send(destination, wireBytes)
	if err != nil {
		return fmt.Errorf("error sending: %v", err)
	}

	return nil
}

func (rh *MessageHandler) send(dst *ecdsa.PublicKey, payload []byte) error {
	return rh.node.Send(MessageParams{
		Payload:     payload,
		Destination: dst,
		Source:      rh.node.key,
	})
}

func (rh *MessageHandler) handleRequest(req *Request) {
	log.Trace("request received", "type", req.Type)
	handler, ok := rh.mappings[req.Type]
	if ok {
		log.Trace("handling message", "id", req.Id)
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

func (rh *MessageHandler) requestWorker(incoming <-chan *Request) {
	for {
		req, ok := <-incoming
		if !ok {
			return
		}
		rh.handleRequest(req)
	}
}

func (rh *MessageHandler) responseWorker(incoming <-chan *Response) {
	for {
		resp, ok := <-incoming
		if !ok {
			return
		}
		rh.handleResponse(resp)
	}
}

func (rh *MessageHandler) Start() {
	if rh.started {
		return
	}
	rh.started = true

	rh.node.Start()
	rh.node.SetHandler(func(bytes []byte) {
		fmt.Print("We've got a response on: %v", bytes)
	})

	for i := 0; i < rh.Concurrency; i++ {
		go rh.responseWorker(rh.responseChannel)
		go rh.requestWorker(rh.requestChannel)
	}

	go func() {
		for {
			select {
			case msg := <-rh.messageChan:
				log.Trace("message received", "msg", msg)
				if msg == nil {
					log.Error("received nil msg")
					continue
				}
				if msg.Request != nil {
					msg.Request.source = msg.source
					rh.requestChannel <- msg.Request
					continue
				}
				if msg.Response != nil {
					msg.Response.source = msg.source
					rh.responseChannel <- msg.Response
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
				// for _, sub := range rh.subs {
				// 	messages := sub.RetrieveMessages()
				// 	for _, msg := range messages {
				// 		log.Trace("message received", "msg", msg)
				// 		rh.messageChan <- messageToWireFormat(msg)
				// 	}
				// }
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
	close(rh.requestChannel)
	close(rh.responseChannel)
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
	sw := &safewrap.SafeWrap{}

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
	sw := &safewrap.SafeWrap{}

	msg := &Message{
		Request: req,
	}

	msgBytes := sw.WrapObject(msg)
	if sw.Err != nil {
		return nil, fmt.Errorf("error wrapping request: %v", sw.Err)
	}

	return msgBytes.RawData(), nil
}
