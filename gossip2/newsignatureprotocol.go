package gossip2

import (
	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/tinylib/msgp/msgp"
)

type SignatureProtocolHandler struct {
	gossipNode *GossipNode
	stream     net.Stream
	reader     *msgp.Reader
	writer     *msgp.Writer
	peerID     string
}

func DoNewSignatureProtocol(gn *GossipNode, stream net.Stream) error {
	sph := &SignatureProtocolHandler{
		gossipNode: gn,
		stream:     stream,
		peerID:     stream.Conn().RemotePeer().Pretty(),
		reader:     msgp.NewReader(stream),
		writer:     msgp.NewWriter(stream),
	}
	log.Debugf("%s received signature protocol request from %s", gn.ID(), sph.peerID)
	defer func() {
		sph.writer.Flush()
		sph.stream.Close()
	}()

	msg, err := sph.ReceiveSignature()
	if err != nil {
		return err
	}
	gn.newObjCh <- msg

	return nil
}

func (sph *SignatureProtocolHandler) ReceiveSignature() (ProvideMessage, error) {
	var msg ProvideMessage
	err := msg.DecodeMsg(sph.reader)

	if err != nil {
		return ProvideMessage{}, err
	}

	return msg, nil
}
