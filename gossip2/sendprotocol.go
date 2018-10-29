package gossip2

import (
	"bytes"
	"context"
	"fmt"

	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/quorumcontrol/qc3/p2p"
	"github.com/tinylib/msgp/msgp"
)

type SyncProtocolHandler struct {
	gossipNode *GossipNode
	stream     net.Stream
	reader     *msgp.Reader
	writer     *msgp.Writer
	peerID     string
}

func DoSyncProtocol(gn *GossipNode) error {
	sph := &SyncProtocolHandler{
		gossipNode: gn,
	}
	log.Debugf("%s starting sync", gn.ID())
	defer func() {
		if sph.writer != nil {
			sph.writer.Flush()
		}
		if sph.stream != nil {
			sph.stream.Close()
		}
	}()

	// Step 1: get a random peer (or use the known next peer)
	stream, err := sph.ConnectToPeer()
	if err != nil {
		return fmt.Errorf("error connecting to peer: %v", err)
	}
	// handle the case where no peer was returned do to a backoff
	if stream == nil {
		return nil
	}

	sph.stream = stream
	sph.writer = msgp.NewWriter(stream)
	sph.reader = msgp.NewReader(stream)
	sph.peerID = stream.Conn().RemotePeer().String()

	// Step 2: Send bloom filter
	err = sph.SendBloomFilter()
	if err != nil {
		return fmt.Errorf("error sending bloom filter: %v", err)
	}

	// Step 3: wait for wants message (terminates here if receives 503)
	wants, err := sph.WaitForWantsMessage()
	if err != nil {
		return fmt.Errorf("error waiting on wants message: %v", err)
	}

	// Step 4: send wanted objects
	err = sph.SendWantedObjects(wants)
	if err != nil {
		return fmt.Errorf("error sending objects: %v", err)
	}

	// step 5: wait for objects we wnant
	err = sph.GetWantedObjects()
	if err != nil {
		return fmt.Errorf("error getting objects: %v", err)
	}

	log.Debugf("%s: sync complete to %s", gn.ID(), sph.peerID)

	return nil
}

func (sph *SyncProtocolHandler) GetWantedObjects() error {
	gn := sph.gossipNode
	reader := sph.reader
	// now get the objects we need
	var isLastMessage bool
	for !isLastMessage {
		var provideMsg ProvideMessage
		err := provideMsg.DecodeMsg(reader)
		if err != nil {
			log.Errorf("%s error decoding message: %v", gn.ID(), err)
			return nil
		}
		if len(provideMsg.Key) == 0 && !provideMsg.Last {
			log.Errorf("%s error, got nil key, value: %v", gn.ID(), provideMsg.Key)
			return nil
		}
		log.Debugf("%s new msg from %s %s %v", gn.ID(), sph.peerID, bytesToString(provideMsg.Key), provideMsg.Key)
		if !provideMsg.Last && len(provideMsg.Key) == 0 {
			log.Errorf("%s provide message has no key: %v", gn.ID(), provideMsg.Key)
		}

		zeroCount := 0
		for i := len(provideMsg.Key) - 1; i >= 0; i-- {
			b := provideMsg.Key[i]
			if !bytes.Equal([]byte{b}, []byte{byte(0)}) {
				break
			}
			zeroCount++
		}
		if zeroCount > 5 {
			log.Errorf("%s all zero key %v provideMessage.Last? %t", gn.ID(), provideMsg.Key, provideMsg.Last)
			panic("we're done in the DoSync")
		}

		gn.newObjCh <- provideMsg
		isLastMessage = provideMsg.Last
	}

	return nil
}

func (sph *SyncProtocolHandler) SendBloomFilter() error {
	gn := sph.gossipNode
	writer := sph.writer

	gn.ibfSyncer.RLock()
	err := gn.IBFs[20000].EncodeMsg(writer)
	gn.ibfSyncer.RUnlock()
	if err != nil {
		return fmt.Errorf("error writing IBF: %v", err)
	}
	log.Debugf("%s flushing", gn.ID())
	return writer.Flush()
}

func (sph *SyncProtocolHandler) SendWantedObjects(wants *WantMessage) error {
	gn := sph.gossipNode
	writer := sph.writer

	for _, key := range wants.Keys {
		key := uint64ToBytes(key)
		objs, err := gn.Storage.GetPairsByPrefix(key)
		if err != nil {
			return fmt.Errorf("error getting objects: %v", err)
		}
		for _, kv := range objs {
			provide := ProvideMessage{
				Key:   kv.Key,
				Value: kv.Value,
			}
			log.Debugf("%s providing to %s (uint64: %d): %s %v", gn.ID(), sph.peerID, key, bytesToString(provide.Key), provide.Key)
			provide.EncodeMsg(writer)
		}
	}
	log.Debugf("%s: sending Last message", gn.ID())
	last := ProvideMessage{Last: true}
	last.EncodeMsg(writer)
	return writer.Flush()
}

func (sph *SyncProtocolHandler) WaitForWantsMessage() (*WantMessage, error) {
	reader := sph.reader
	gn := sph.gossipNode
	var wants WantMessage
	err := wants.DecodeMsg(reader)
	if err != nil {
		log.Errorf("%s error reading wants %v", gn.ID(), err)
		// log.Errorf("%s ibf TO %s : %v", gn.ID(), peerID, gn.IBFs[2000].GetDebug())
		// log.Errorf("%s ibf TO %s cells : %v", gn.ID(), peerID, ibf.HumanizeIBF(gn.IBFs[2000]))

		return nil, fmt.Errorf("error decoding wants: %v", err)
	}
	log.Debugf("%s: got a want request for %v keys", gn.ID(), len(wants.Keys))
	if wants.Code == 503 {
		log.Infof("%s remote peer %s too busy", gn.ID(), sph.peerID)
		return nil, fmt.Errorf("peer too busy")
	}
	return &wants, nil
}

func (sph *SyncProtocolHandler) ConnectToPeer() (net.Stream, error) {
	gn := sph.gossipNode

	peer, err := gn.getSyncTarget()
	if err != nil {
		return nil, fmt.Errorf("error getting peer: %v", err)
	}
	peerPublicKey := bytesToString(peer.DstKey.PublicKey)[0:8]

	log.Debugf("%s: targeting peer %v", gn.ID(), peerPublicKey)

	ctx := context.Background()
	stream, err := gn.Host.NewStream(ctx, peer.DstKey.ToEcdsaPub(), syncProtocol)
	if err != nil {
		if err == p2p.ErrDialBackoff {
			log.Debugf("%s: dial backoff for peer %s", gn.ID(), peerPublicKey)
			return nil, nil
		}
		return nil, fmt.Errorf("%s: error opening new stream - %v", gn.ID(), err)
	}
	log.Debugf("%s established stream to %s", gn.ID(), sph.peerID)
	return stream, nil
}
