package gossip2

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/quorumcontrol/differencedigest/ibf"
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
	atomic.AddUint64(&gn.debugAttemptSync, 1)

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

	// step 1 send strata
	err = sph.SendStrata()
	if err != nil {
		return fmt.Errorf("error sending strata: %v", err)
	}

	// step 2 wait for IBF (error if received 503, terminate if 304)
	var remoteDiff *ibf.DecodeResults

	code, remoteFilter, err := sph.WaitForBloomFilter()
	if err != nil {
		return fmt.Errorf("error waiting for bloom filter: %v", err)
	}
	switch code {
	case 503:
		log.Debugf("%s remote side too busy %s", gn.ID(), sph.peerID)
		return nil
	case 304:
		log.Debugf("%s full synced with %s", gn.ID(), sph.peerID)
		return nil
	case 302:
		// 302 means the other side decoded our strata
		// and will instead send a wants message
		wants, err := sph.WaitForWantsMessage()
		if err != nil {
			return fmt.Errorf("error waiting for wants")
		}
		var dif []ibf.ObjectId
		for _, key := range wants.Keys {
			dif = append(dif, ibf.ObjectId(key))
		}
		remoteDiff = &ibf.DecodeResults{
			LeftSet: dif,
		}
	default:
		// step 3 - decode IBF
		remoteDiff, err = sph.DifferencesFromBloomFilter(remoteFilter)
		if err != nil {
			log.Infof("sending error message: error getting differences from bloom: %v", err)
			err = sph.SendErrorMessage(416, fmt.Sprintf("error getting differences from bloom: %v", err))
			return err
		}
		// step 4 - send wants
		_, err = sph.SendWantMessage(remoteDiff)
		if err != nil {
			return fmt.Errorf("error sending want message: %v", err)
		}
	}

	// step 5 - handle incoming obects and send wanted objects

	wg := &sync.WaitGroup{}
	results := make(chan error, 2)

	wg.Add(1)
	go func() {
		results <- sph.SendPeerObjects(remoteDiff)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		results <- sph.WaitForProvides()
		wg.Done()
	}()
	wg.Wait()
	close(results)
	for res := range results {
		if res != nil {
			return fmt.Errorf("error sending or receiving: %v", err)
		}
	}

	log.Debugf("%s: sync complete to %s", gn.ID(), sph.peerID)
	atomic.AddUint64(&gn.debugSendSync, 1)

	return nil
}

func (sph *SyncProtocolHandler) SendErrorMessage(code int, msg string) error {
	writer := sph.writer
	pm := &ProtocolMessage{
		Code:  code,
		Error: msg,
	}
	return pm.EncodeMsg(writer)
}

func (sph *SyncProtocolHandler) DifferencesFromBloomFilter(remoteIBF *ibf.InvertibleBloomFilter) (*ibf.DecodeResults, error) {
	// log.Debugf("%s from %s received IBF (cells)", gn.ID(), peerID, ibf.HumanizeIBF(&remoteIBF))
	gn := sph.gossipNode
	gn.ibfSyncer.RLock()
	subtracted := gn.IBFs[len(remoteIBF.Cells)].Subtract(remoteIBF)
	// debug := gn.IBFs[len(remoteIBF.Cells)].GetDebug()
	gn.ibfSyncer.RUnlock()
	difference, err := subtracted.Decode()
	if err != nil {
		log.Infof("%s error getting diff from peer %s (remote size: %d): %v", gn.ID(), sph.peerID, len(remoteIBF.Cells), err)
		// log.Errorf("%s local IBF on error from %s is: %v", gn.ID(), sph.peerID, debug)
		// log.Errorf("%s (talking to %s) local ibf cells : %v", gn.ID(), peerID, ibf.HumanizeIBF(gn.IBFs[2000]))
		return nil, err
	}
	log.Debugf("%s decoded", gn.ID())
	return &difference, nil
}

func (sph *SyncProtocolHandler) SendWantMessage(difference *ibf.DecodeResults) (*WantMessage, error) {
	writer := sph.writer
	gn := sph.gossipNode

	want := WantMessageFromDiff(difference.RightSet)
	pm, err := ToProtocolMessage(want)
	if err != nil {
		return nil, fmt.Errorf("error writing wants: %v", err)
	}
	err = pm.EncodeMsg(writer)
	if err != nil {
		log.Errorf("%s error writing wants protocol message: %v", gn.ID(), err)
		return nil, fmt.Errorf("%s error writing wants protocol message: %v", gn.ID(), err)
	}
	writer.Flush()
	return want, nil
}

func (sph *SyncProtocolHandler) WaitForWantsMessage() (*WantMessage, error) {
	reader := sph.reader

	var wants WantMessage
	err := wants.DecodeMsg(reader)
	if err != nil {
		return nil, fmt.Errorf("error reading wants: %v", err)
	}
	return &wants, nil
}

func (sph *SyncProtocolHandler) WaitForBloomFilter() (int, *ibf.InvertibleBloomFilter, error) {
	reader := sph.reader
	gn := sph.gossipNode
	var pm ProtocolMessage
	err := pm.DecodeMsg(reader)
	if err != nil {
		log.Errorf("%s error decoding message: %v", gn.ID(), err)
		return 500, nil, fmt.Errorf("error decoding message: %v", err)
	}
	log.Debugf("%s bloom filter code %d from %s", gn.ID(), pm.Code, sph.peerID)

	switch pm.Code {
	case 503:
		log.Debugf("error too many syncs on remote side")
		return 503, nil, nil
	case 304:
		return 304, nil, nil
	case 302:
		return 302, nil, nil
	default:
		remoteIBF, err := FromProtocolMessage(&pm)
		if err != nil {
			return 500, nil, fmt.Errorf("error converting pm to remoteIBF: %v", err)
		}
		return 200, remoteIBF.(*ibf.InvertibleBloomFilter), nil
	}
}

func (sph *SyncProtocolHandler) SendPeerObjects(difference *ibf.DecodeResults) error {
	gn := sph.gossipNode
	writer := sph.writer

	wantedKeys := difference.LeftSet

	toProvideAsWantMessage := WantMessageFromDiff(wantedKeys)
	for _, key := range toProvideAsWantMessage.Keys {
		key := uint64ToBytes(key)
		objs, err := gn.Storage.GetPairsByPrefix(key)
		if err != nil {
			log.Errorf("error getting objects: %v", err)
			return fmt.Errorf("error getting objects: %v", err)
		}
		for _, kv := range objs {
			provide := ProvideMessage{
				Key:   kv.Key,
				Value: kv.Value,
			}
			log.Debugf("%s HandleSync providing to %s (uint64: %d): %s %v", gn.ID(), sph.peerID, key, bytesToString(provide.Key), provide.Key)
			provide.EncodeMsg(writer)
			writer.Flush()
		}
	}
	last := ProvideMessage{Last: true}
	last.EncodeMsg(writer)
	return nil
}

func (sph *SyncProtocolHandler) SendStrata() error {
	gn := sph.gossipNode
	writer := sph.writer

	gn.ibfSyncer.RLock()
	err := gn.Strata.EncodeMsg(writer)
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

// TODO: handle 503
// log.Debugf("%s: got a want request for %v keys", gn.ID(), len(wants.Keys))
// if wants.Code == 503 {
// 	log.Infof("%s remote peer %s too busy", gn.ID(), rsph.peerID)
// 	return nil, fmt.Errorf("peer too busy")
// }

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
		return nil, fmt.Errorf("error opening new stream to %s - %v", peerPublicKey, err)
	}
	log.Debugf("%s established stream to %s", gn.ID(), sph.peerID)
	return stream, nil
}

func (sph *SyncProtocolHandler) WaitForProvides() error {
	gn := sph.gossipNode
	reader := sph.reader
	var isLastMessage bool
	for !isLastMessage {
		var provideMsg ProvideMessage
		err := provideMsg.DecodeMsg(reader)
		if err != nil {
			log.Errorf("%s error decoding message: %v", gn.ID(), err)
			return fmt.Errorf("error decoding message: %v", err)
		}
		log.Debugf("%s HandleSync new msg from %s %s %v", gn.ID(), sph.peerID, bytesToString(provideMsg.Key), provideMsg.Key)
		isLastMessage = provideMsg.Last
		if !isLastMessage {
			gn.newObjCh <- provideMsg
		}
	}

	log.Debugf("%s: HandleSync received all provides from %s, moving forward", gn.ID(), sph.peerID)
	return nil
}
