package network

import (
	"crypto/ecdsa"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/dag"
	"sync"
	"time"
)

type Client struct {
	node                *Node
	key                 *ecdsa.PrivateKey
	outstandingRequests map[string]chan *Response
	lock                *sync.Mutex
	stopChan            chan bool
	topic               []byte
	symkey              []byte
}

func NewClient(key *ecdsa.PrivateKey, topic []byte, symkey []byte) *Client {
	node := NewNode(key)
	return &Client{
		node:                node,
		key:                 key,
		outstandingRequests: make(map[string]chan *Response),
		stopChan:            make(chan bool, 1),
		topic:               topic,
		symkey:              symkey,
		lock:                &sync.Mutex{},
	}
}

func (c *Client) Start() {
	c.node.Start()
	sub := c.node.SubscribeToKey(c.key)

	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)

		for {
			select {
			case <-ticker.C:
				messages := sub.RetrieveMessages()
				for _, msg := range messages {
					resp := &Response{}
					err := cbornode.DecodeInto(msg.Payload, resp)
					if err != nil {
						log.Error("error decoding", "err", err)
						break
					}

					outChan, ok := c.outstandingRequests[resp.Id]
					if ok {
						outChan <- resp
						close(outChan)
						c.lock.Lock()
						delete(c.outstandingRequests, resp.Id)
						c.lock.Unlock()
					} else {
						log.Error("received response for unknown message", "id", resp.Id)
					}
				}
			case <-c.stopChan:
				return
			}
		}
	}()
}

func (c *Client) Stop() {
	c.stopChan <- true
	c.node.Stop()
}

func (c *Client) DoRequest(req *Request) (chan *Response, error) {
	respChan := make(chan *Response)
	c.lock.Lock()
	c.outstandingRequests[req.Id] = respChan
	c.lock.Unlock()

	sw := &dag.SafeWrap{}
	reqNode := sw.WrapObject(req)
	if sw.Err != nil {
		log.Error("error wrapping request", "err", sw.Err)
		return nil, fmt.Errorf("error wrapping request: %v", sw.Err)
	}

	log.Debug("sending message")
	err := c.node.Send(MessageParams{
		Payload: reqNode.RawData(),
		TTL:     DefaultTTL,
		PoW:     0.02,
		KeySym:  c.symkey,
		Topic:   c.topic,
		Src:     c.node.key,
	})

	if err != nil {
		return nil, fmt.Errorf("error sending: %v", err)
	}

	return respChan, nil
}
