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
	// step 2 wait for IBF
	remoteFilter, err := sph.WaitForBloomFilter()
	if err != nil {
		return fmt.Errorf("error waiting for bloom filter: %v", err)
	}

	// step 3 - decode IBF
	diff, err := sph.DifferencesFromBloomFilter(remoteFilter)
	if err != nil {
		return fmt.Errorf("error getting differences from bloom: %v", err)
	}
	// step 4 - send wants
	_, err = sph.SendWantMessage(diff)
	if err != nil {
		return fmt.Errorf("error sending want message: %v", err)
	}

	// step 5 - handle incoming obects and send wanted objects

	wg := &sync.WaitGroup{}
	results := make(chan error, 2)

	wg.Add(1)
	go func() {
		results <- sph.SendPeerObjects(diff)
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

func (sph *SyncProtocolHandler) DifferencesFromBloomFilter(remoteIBF *ibf.InvertibleBloomFilter) (*ibf.DecodeResults, error) {
	// log.Debugf("%s from %s received IBF (cells)", gn.ID(), peerID, ibf.HumanizeIBF(&remoteIBF))
	gn := sph.gossipNode
	gn.ibfSyncer.RLock()
	subtracted := gn.IBFs[len(remoteIBF.Cells)].Subtract(remoteIBF)
	gn.ibfSyncer.RUnlock()
	difference, err := subtracted.Decode()
	if err != nil {
		log.Errorf("%s error getting diff (remote size: %d): %v", gn.ID(), len(remoteIBF.Cells), err)
		// log.Errorf("%s (talking to %s) local ibf is : %v", gn.ID(), peerID, gn.IBFs[2000].GetDebug())
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
	err := want.EncodeMsg(writer)
	if err != nil {
		log.Errorf("%s error writing wants: %v", gn.ID(), err)
		return nil, fmt.Errorf("")
	}
	writer.Flush()
	return want, nil
}

func (sph *SyncProtocolHandler) WaitForBloomFilter() (*ibf.InvertibleBloomFilter, error) {
	reader := sph.reader
	gn := sph.gossipNode

	var remoteIBF ibf.InvertibleBloomFilter
	err := remoteIBF.DecodeMsg(reader)
	if err != nil {
		log.Errorf("%s error decoding message: %v", gn.ID(), err)
		return &remoteIBF, fmt.Errorf("error decoding message: %v", err)
	}
	return &remoteIBF, nil
}

func (sph *SyncProtocolHandler) SendPeerObjects(difference *ibf.DecodeResults) error {
	gn := sph.gossipNode
	writer := sph.writer

	wantedKeys := difference.LeftSet
	if useDoneRemovalFilter {
		wantedKeys = make([]ibf.ObjectId, 0)
		for _, oid := range difference.LeftSet {
			key := uint64ToBytes(uint64(oid))
			done, _ := gn.isDone(key)
			if !done {
				wantedKeys = append(wantedKeys, oid)
			}
		}
	}

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
		return nil, fmt.Errorf("%s: error opening new stream - %v", gn.ID(), err)
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
		gn.newObjCh <- provideMsg
		isLastMessage = provideMsg.Last
	}

	log.Debugf("%s: HandleSync received all provides from %s, moving forward", gn.ID(), sph.peerID)
	return nil
}
