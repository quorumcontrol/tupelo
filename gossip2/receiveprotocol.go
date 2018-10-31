package gossip2

import (
	"fmt"
	"sync"
	"sync/atomic"

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

	// step 1 wait for strata
	strata, err := rsph.WaitForStrata()
	if err != nil {
		return fmt.Errorf("error waiting for strata: %v", err)
	}

	// step 2 estimate strata
	estimate, _ := rsph.EstimateFromRemoteStrata(strata)

	// step 3 - send appropriate IBF
	err = rsph.SendBloomFilter(estimate)
	if err != nil {
		return fmt.Errorf("error sending bloom filter: %v", err)
	}
	// step 4 - wait for wants
	wants, err := rsph.WaitForWantsMessage()
	if err != nil {
		return fmt.Errorf("error waiting on wants message: %v", err)
	}

	// step 5 - handle incoming obects and send wanted objects

	wg := &sync.WaitGroup{}
	results := make(chan error, 2)

	wg.Add(1)
	go func() {
		// Step 4: handle incoming objects
		results <- rsph.WaitForProvides()
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		results <- rsph.SendWantedObjects(wants)
		wg.Done()
	}()
	wg.Wait()
	close(results)
	for res := range results {
		if res != nil {
			return fmt.Errorf("error sending or waiting: %v", err)
		}
	}

	atomic.AddUint64(&gn.debugReceiveSync, 1)
	return nil
}

func (rsph *ReceiveSyncProtocolHandler) Send503IfTooManyInProgress() (*SyncHandlerWorker, error) {
	gn := rsph.gossipNode
	select {
	case worker := <-gn.syncPool:
		return &worker, nil
	default:
		// TODO: for now just do nothing, but we should probably write something here
		// writer := rsph.writer
		// want := &WantMessage{
		// 	Code: 503,
		// }
		// err := want.EncodeMsg(rsph.writer)
		// if err != nil {
		// 	log.Errorf("%s error writing wants: %v", gn.ID(), err)
		// 	return nil, fmt.Errorf("error writing wants: %v", err)
		// }
		return nil, nil
	}
}

func (rsph *ReceiveSyncProtocolHandler) WaitForWantsMessage() (*WantMessage, error) {
	reader := rsph.reader
	gn := rsph.gossipNode
	var wants WantMessage
	err := wants.DecodeMsg(reader)
	if err != nil {
		log.Errorf("%s error reading wants %v", gn.ID(), err)
		// log.Errorf("%s ibf TO %s : %v", gn.ID(), peerID, gn.IBFs[2000].GetDebug())
		// log.Errorf("%s ibf TO %s cells : %v", gn.ID(), peerID, ibf.HumanizeIBF(gn.IBFs[2000]))

		return nil, fmt.Errorf("error decoding wants: %v", err)
	}

	return &wants, nil
}

func (rsph *ReceiveSyncProtocolHandler) SendBloomFilter(estimate int) error {
	//TODO: send *correct bloom filter
	gn := rsph.gossipNode
	writer := rsph.writer

	gn.ibfSyncer.RLock()
	err := gn.IBFs[20000].EncodeMsg(writer)
	gn.ibfSyncer.RUnlock()
	if err != nil {
		return fmt.Errorf("error writing IBF: %v", err)
	}
	log.Debugf("%s flushing", gn.ID())
	return writer.Flush()
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

func (rsph *ReceiveSyncProtocolHandler) WaitForStrata() (*ibf.DifferenceStrata, error) {
	reader := rsph.reader
	gn := rsph.gossipNode

	var remoteStrata ibf.DifferenceStrata
	err := remoteStrata.DecodeMsg(reader)
	if err != nil {
		log.Errorf("%s error decoding message", gn.ID(), err)
		return &remoteStrata, fmt.Errorf("error decoding message: %v", err)
	}
	return &remoteStrata, nil
}

func (rsph *ReceiveSyncProtocolHandler) EstimateFromRemoteStrata(remoteStrata *ibf.DifferenceStrata) (count int, result *ibf.DecodeResults) {
	// log.Debugf("%s from %s received IBF (cells)", gn.ID(), peerID, ibf.HumanizeIBF(&remoteIBF))
	gn := rsph.gossipNode
	gn.ibfSyncer.RLock()
	count, result = gn.Strata.Estimate(remoteStrata)
	gn.ibfSyncer.RUnlock()
	return count, result
}

func (rsph *ReceiveSyncProtocolHandler) SendWantedObjects(wants *WantMessage) error {
	gn := rsph.gossipNode
	writer := rsph.writer

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
			log.Debugf("%s providing to %s (uint64: %d): %s %v", gn.ID(), rsph.peerID, key, bytesToString(provide.Key), provide.Key)
			provide.EncodeMsg(writer)
		}
	}
	log.Debugf("%s: sending Last message", gn.ID())
	last := ProvideMessage{Last: true}
	last.EncodeMsg(writer)
	return writer.Flush()
}
