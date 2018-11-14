package gossip2

import (
	"fmt"
	"sync"
	"time"

	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/tinylib/msgp/msgp"
)

type subscriptionHolder struct {
	objs  map[string][]chan CurrentState
	count uint64
	lock  *sync.Mutex
}

func newSubscriptionHolder() *subscriptionHolder {
	return &subscriptionHolder{
		lock: &sync.Mutex{},
		objs: make(map[string][]chan CurrentState),
	}
}

func (sh *subscriptionHolder) Subscribe(objectID []byte) <-chan CurrentState {
	ch := make(chan CurrentState, 1)
	key := string(objectID)
	sh.lock.Lock()
	chans := sh.objs[key]
	sh.objs[key] = append(chans, ch)
	sh.count++
	sh.lock.Unlock()
	return ch
}

func (sh *subscriptionHolder) Notify(objectID []byte, state CurrentState) {
	sh.lock.Lock()
	for _, ch := range sh.objs[string(objectID)] {
		ch <- state
		close(ch)
		sh.count--
	}
	delete(sh.objs, string(objectID))
	sh.lock.Unlock()
}

type ChainTreeChangeProtocolHandler struct {
	gossipNode *GossipNode
	stream     net.Stream
	reader     *msgp.Reader
	writer     *msgp.Writer
	peerID     string
}

const MaxSubscriberCount = 200

func DoChainTreeChangeProtocol(gn *GossipNode, stream net.Stream) error {
	ctcp := &ChainTreeChangeProtocolHandler{
		gossipNode: gn,
		stream:     stream,
		peerID:     stream.Conn().RemotePeer().String(),
		reader:     msgp.NewReader(stream),
		writer:     msgp.NewWriter(stream),
	}
	timer := time.NewTimer(5 * time.Minute)
	defer func() {
		ctcp.writer.Flush()
		stream.Close()
	}()

	if gn.subscriptions.count > MaxSubscriberCount {
		return ctcp.Send503()
	}

	objID, err := ctcp.ReceiveObjectID()
	if err != nil {
		return fmt.Errorf("error receiving objectID: %v", err)
	}

	ch := gn.subscriptions.Subscribe(objID)

	state, err := ctcp.currentStateFromStorage(objID)
	if err != nil {
		return fmt.Errorf("error getting current state from storage: %v", err)
	}

	err = ctcp.sendCurrentState(state)
	if err != nil {
		return fmt.Errorf("error receiving objectID: %v", err)
	}
	select {
	case currentState := <-ch:
		err = ctcp.sendCurrentState(currentState)
		if err != nil {
			return fmt.Errorf("error getting current state from storage")
		}
	case <-timer.C:
		return fmt.Errorf("timeout")
	}

	return nil
}

func (ctcp *ChainTreeChangeProtocolHandler) Send503() error {
	pm := &ProtocolMessage{
		Code: 503,
	}
	err := pm.EncodeMsg(ctcp.writer)
	if err != nil {
		log.Errorf("%s error writing wants: %v", ctcp.gossipNode.ID(), err)
		return fmt.Errorf("error writing wants: %v", err)
	}
	return nil
}

func (ctcp *ChainTreeChangeProtocolHandler) ReceiveObjectID() ([]byte, error) {
	var req ChainTreeSubscriptionRequest
	err := req.DecodeMsg(ctcp.reader)

	if err != nil {
		return nil, err
	}

	return req.ObjectID, nil
}

func (ctcp *ChainTreeChangeProtocolHandler) currentStateFromStorage(objectID []byte) (CurrentState, error) {
	var currentState CurrentState
	objBytes, err := ctcp.gossipNode.Storage.Get(objectID)

	if err != nil {
		return CurrentState{}, fmt.Errorf("error getting ObjectID (%s): %v", objectID, err)
	}
	if len(objBytes) > 0 {
		_, err = currentState.UnmarshalMsg(objBytes)
		if err != nil {
			return CurrentState{}, fmt.Errorf("error unmarshaling: %v", err)
		}
	}
	return currentState, nil
}

func (ctcp *ChainTreeChangeProtocolHandler) sendCurrentState(currentState CurrentState) error {
	pm, err := ToProtocolMessage(&currentState)
	if err != nil {
		return err
	}
	pm.Code = 200
	return pm.EncodeMsg(ctcp.writer)
}
