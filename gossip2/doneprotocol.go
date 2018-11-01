package gossip2

import (
	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/tinylib/msgp/msgp"
)

type DoneProtocolHandler struct {
	gossipNode *GossipNode
	stream     net.Stream
	reader     *msgp.Reader
	writer     *msgp.Writer
	peerID     string
}

func DoDoneProtocol(gn *GossipNode, stream net.Stream) error {
	dph := &DoneProtocolHandler{
		gossipNode: gn,
		stream:     stream,
		peerID:     stream.Conn().RemotePeer().String(),
		reader:     msgp.NewReader(stream),
		writer:     msgp.NewWriter(stream),
	}
	log.Debugf("%s received done protocol request from %s", gn.ID(), dph.peerID)
	defer func() {
		dph.writer.Flush()
		dph.stream.Close()
	}()

	csq, err := dph.ReceiveTransaction()
	if err != nil {
		return err
	}

	err = dph.SendResponse(csq)
	if err != nil {
		return err
	}

	return nil
}

func (dph *DoneProtocolHandler) ReceiveTransaction() (ConflictSetQuery, error) {
	var csq ConflictSetQuery
	err := csq.DecodeMsg(dph.reader)

	if err != nil {
		return ConflictSetQuery{}, err
	}

	return csq, nil
}

func (dph *DoneProtocolHandler) SendResponse(csq ConflictSetQuery) error {
	csqr := ConflictSetQueryResponse{
		Key: csq.Key,
	}

	done, err := isDone(dph.gossipNode.Storage, csq.Key)
	if err != nil {
		return err
	}

	csqr.Done = done

	return csqr.EncodeMsg(dph.writer)
}
