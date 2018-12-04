package gossip2

import (
	"fmt"

	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/tinylib/msgp/msgp"
)

type TipProtocolHandler struct {
	gossipNode *GossipNode
	stream     net.Stream
	reader     *msgp.Reader
	writer     *msgp.Writer
	peerID     string
}

func DoTipProtocol(gn *GossipNode, stream net.Stream) error {
	tph := &TipProtocolHandler{
		gossipNode: gn,
		stream:     stream,
		peerID:     stream.Conn().RemotePeer().Pretty(),
		reader:     msgp.NewReader(stream),
		writer:     msgp.NewWriter(stream),
	}
	log.Debugf("%s received done protocol request from %s", gn.ID(), tph.peerID)
	defer func() {
		tph.writer.Flush()
		tph.stream.Close()
	}()

	tq, err := tph.ReceiveTipQuery()
	if err != nil {
		return err
	}

	err = tph.SendResponse(tq)
	if err != nil {
		return err
	}

	return nil
}

func (tph *TipProtocolHandler) ReceiveTipQuery() (TipQuery, error) {
	var tq TipQuery
	err := tq.DecodeMsg(tph.reader)

	if err != nil {
		return TipQuery{}, err
	}

	return tq, nil
}

func (tph *TipProtocolHandler) SendResponse(tq TipQuery) error {
	gn := tph.gossipNode
	objBytes, err := gn.Storage.Get(tq.ObjectID)
	if err != nil {
		return fmt.Errorf("error getting ObjectID (%s): %v", tq.ObjectID, err)
	}
	var currentState CurrentState
	if len(objBytes) > 0 {
		_, err = currentState.UnmarshalMsg(objBytes)
		if err != nil {
			return fmt.Errorf("error unmarshaling: %v", err)
		}
	} else {
		log.Errorf("could not find current state")
	}
	return currentState.EncodeMsg(tph.writer)
}
