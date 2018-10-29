package gossip2

import (
	"fmt"

	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/quorumcontrol/differencedigest/ibf"
	"github.com/tinylib/msgp/msgp"
)

type ReceiveSyncProtocolHandler struct {
	gossipNode *GossipNode
	stream     net.Stream
	reader     *msgp.Reader
	writer     *msgp.Writer
	peerID     string
}

func DoReceiveSyncProtocol(gn *GossipNode, stream net.Stream) error {
	rsph := &ReceiveSyncProtocolHandler{
		gossipNode: gn,
		stream:     stream,
		peerID:     stream.Conn().RemotePeer().String(),
		reader:     msgp.NewReader(stream),
		writer:     msgp.NewWriter(stream),
	}
	log.Debugf("%s received sync request from %s", gn.ID(), rsph.peerID)
	defer func() {
		rsph.writer.Flush()
		rsph.stream.Close()
	}()

	// Step 0 (terminate if too busy)
	// If we are already processing NumberOfSyncWorkers syncs, then send a 503
	worker, err := rsph.Send503IfTooManyInProgress()
	if err != nil {
		return fmt.Errorf("error sending 503: %v", err)
	}
	if worker == nil {
		return nil
	}
	defer func() { gn.syncPool <- *worker }()

	// Step 1: wait for a bloom filter
	remoteFilter, err := rsph.WaitForBloomFilter()
	if err != nil {
		return fmt.Errorf("error waiting for bloom filter: %v", err)
	}

	// Step 2: decode remote bloom filter
	diff, err := rsph.DifferencesFromBloomFilter(remoteFilter)
	if err != nil {
		return fmt.Errorf("error getting differences from bloom: %v", err)
	}

	// Step 3: send a want message to the remote side
	err = rsph.SendWantMessage(diff)
	if err != nil {
		return fmt.Errorf("error sending want message: %v", err)
	}

	// Step 4: handle incoming objects
	err = rsph.WaitForProvides()
	if err != nil {
		return fmt.Errorf("error waiting for provides: %v", err)
	}

	err = rsph.SendPeerObjects(diff)
	if err != nil {
		return fmt.Errorf("error sending peer objects: %v", err)
	}
	return nil
}

func (rsph *ReceiveSyncProtocolHandler) Send503IfTooManyInProgress() (*SyncHandlerWorker, error) {
	gn := rsph.gossipNode
	select {
	case worker := <-gn.syncPool:
		return &worker, nil
	default:
		writer := rsph.writer
		want := &WantMessage{
			Code: 503,
		}
		err := want.EncodeMsg(rsph.writer)
		if err != nil {
			log.Errorf("%s error writing wants: %v", gn.ID(), err)
			return nil, fmt.Errorf("error writing wants: %v", err)
		}
		return nil, writer.Flush()
	}
}

func (rsph *ReceiveSyncProtocolHandler) SendPeerObjects(difference *ibf.DecodeResults) error {
	gn := rsph.gossipNode
	writer := rsph.writer

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
			log.Debugf("%s HandleSync providing to %s (uint64: %d): %s %v", gn.ID(), rsph.peerID, key, bytesToString(provide.Key), provide.Key)
			provide.EncodeMsg(writer)
			writer.Flush()
		}
	}
	last := ProvideMessage{Last: true}
	last.EncodeMsg(writer)
	return nil
}

func (rsph *ReceiveSyncProtocolHandler) WaitForProvides() error {
	gn := rsph.gossipNode
	reader := rsph.reader
	var isLastMessage bool
	for !isLastMessage {
		var provideMsg ProvideMessage
		err := provideMsg.DecodeMsg(reader)
		if err != nil {
			log.Errorf("%s error decoding message: %v", gn.ID(), err)
			return fmt.Errorf("error decoding message: %v", err)
		}
		log.Debugf("%s HandleSync new msg from %s %s %v", gn.ID(), rsph.peerID, bytesToString(provideMsg.Key), provideMsg.Key)
		gn.newObjCh <- provideMsg
		isLastMessage = provideMsg.Last
	}

	log.Debugf("%s: HandleSync received all provides from %s, moving forward", gn.ID(), rsph.peerID)
	return nil
}

func (rsph *ReceiveSyncProtocolHandler) SendWantMessage(difference *ibf.DecodeResults) error {
	writer := rsph.writer
	gn := rsph.gossipNode

	want := WantMessageFromDiff(difference.RightSet)
	err := want.EncodeMsg(writer)
	if err != nil {
		log.Errorf("%s error writing wants: %v", gn.ID(), err)
		return fmt.Errorf("")
	}
	writer.Flush()
	return nil
}

func (rsph *ReceiveSyncProtocolHandler) DifferencesFromBloomFilter(remoteIBF *ibf.InvertibleBloomFilter) (*ibf.DecodeResults, error) {
	// log.Debugf("%s from %s received IBF (cells)", gn.ID(), peerID, ibf.HumanizeIBF(&remoteIBF))
	gn := rsph.gossipNode
	gn.ibfSyncer.RLock()
	subtracted := gn.IBFs[20000].Subtract(remoteIBF)
	gn.ibfSyncer.RUnlock()
	difference, err := subtracted.Decode()
	if err != nil {
		log.Errorf("%s error getting diff): %f\n", gn.ID(), err)
		// log.Errorf("%s (talking to %s) local ibf is : %v", gn.ID(), peerID, gn.IBFs[2000].GetDebug())
		// log.Errorf("%s (talking to %s) local ibf cells : %v", gn.ID(), peerID, ibf.HumanizeIBF(gn.IBFs[2000]))
		return nil, err
	}
	log.Debugf("%s decoded", gn.ID())
	return &difference, nil
}

func (rsph *ReceiveSyncProtocolHandler) WaitForBloomFilter() (*ibf.InvertibleBloomFilter, error) {
	reader := rsph.reader
	gn := rsph.gossipNode

	var remoteIBF ibf.InvertibleBloomFilter
	err := remoteIBF.DecodeMsg(reader)
	if err != nil {
		log.Errorf("%s error decoding message", gn.ID(), err)
		return &remoteIBF, fmt.Errorf("error decoding message: %v", err)
	}
	return &remoteIBF, nil
}
