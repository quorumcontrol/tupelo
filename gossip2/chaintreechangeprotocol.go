package gossip2

import (
	"fmt"

	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/tinylib/msgp/msgp"
)

type ChainTreeSubscription struct {
	objectID   []byte
	gossipNode *GossipNode
	stream     net.Stream
	reader     *msgp.Reader
	writer     *msgp.Writer
	peerID     string
}

func DoChainTreeChangeProtocol(gn *GossipNode, stream net.Stream) (*ChainTreeSubscription, error) {
	subscription := &ChainTreeSubscription{
		gossipNode: gn,
		stream:     stream,
		peerID:     stream.Conn().RemotePeer().String(),
		reader:     msgp.NewReader(stream),
		writer:     msgp.NewWriter(stream),
	}
	log.Debugf("%s received ChainTreeChange protocol request from %s", gn.ID(), subscription.peerID)

	err := subscription.Start()
	return subscription, err
}

func (s *ChainTreeSubscription) Start() error {
	var req ChainTreeSubscriptionRequest
	err := req.DecodeMsg(s.reader)

	if err != nil {
		return err
	}

	s.objectID = req.ObjectID

	err = s.sendCurrentState()
	if err != nil {
		return err
	}

	return nil
}

func (s *ChainTreeSubscription) Finish() error {
	defer func() {
		s.writer.Flush()
		s.stream.Close()
	}()
	return s.sendCurrentState()
}

func (s *ChainTreeSubscription) sendCurrentState() error {
	var currentState CurrentState
	objBytes, err := s.gossipNode.Storage.Get(s.objectID)

	if err != nil {
		return fmt.Errorf("error getting ObjectID (%s): %v", s.objectID, err)
	}
	if len(objBytes) > 0 {
		_, err = currentState.UnmarshalMsg(objBytes)
		if err != nil {
			return fmt.Errorf("error unmarshaling: %v", err)
		}
	}

	return currentState.EncodeMsg(s.writer)
}
