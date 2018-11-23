package gossip2

import (
	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/tinylib/msgp/msgp"
)

type NewTransactionProtocolHandler struct {
	gossipNode *GossipNode
	stream     net.Stream
	reader     *msgp.Reader
	writer     *msgp.Writer
	peerID     string
}

func DoNewTransactionProtocol(gn *GossipNode, stream net.Stream) error {
	ntph := &NewTransactionProtocolHandler{
		gossipNode: gn,
		stream:     stream,
		peerID:     stream.Conn().RemotePeer().String(),
		reader:     msgp.NewReader(stream),
		writer:     msgp.NewWriter(stream),
	}
	log.Debugf("%s received done protocol request from %s", gn.ID(), ntph.peerID)
	defer func() {
		ntph.writer.Flush()
		ntph.stream.Close()
	}()

	transaction, err := ntph.ReceiveTransaction()

	if err != nil {
		return err
	}

	_, err = ntph.gossipNode.InitiateTransaction(transaction)
	return err
}

func (ntph *NewTransactionProtocolHandler) ReceiveTransaction() (Transaction, error) {
	var transaction Transaction
	err := transaction.DecodeMsg(ntph.reader)

	if err != nil {
		return Transaction{}, err
	}

	return transaction, nil
}
