package gossip2

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	opentracing "github.com/opentracing/opentracing-go"
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
		peerID:     stream.Conn().RemotePeer().Pretty(),
		reader:     msgp.NewReader(stream),
		writer:     msgp.NewWriter(stream),
	}
	log.Debugf("%s received sync request from %s", gn.ID(), rsph.peerID)
	defer func() {
		rsph.writer.Flush()
		rsph.stream.Close()
	}()

	span, ctx := newSpan(context.Background(), gn.Tracer, "DoReceiveSyncProtocol")
	span.SetTag("gn", gn.ID())
	span.SetTag("gsender", rsph.peerID)
	defer span.Finish()

	// Step 0 (terminate if too busy)
	// If we are already processing NumberOfSyncWorkers syncs, then send a 503
	worker, err := rsph.Send503IfTooManyInProgress(ctx)
	if err != nil {
		return fmt.Errorf("error sending 503: %v", err)
	}
	if worker == nil {
		return nil
	}
	defer func() { gn.syncPool <- *worker }()

	// step 1 wait for strata
	strata, err := rsph.WaitForStrata(ctx)
	if err != nil {
		return fmt.Errorf("error waiting for strata: %v", err)
	}

	// step 2 estimate strata
	estimate, diff := rsph.EstimateFromRemoteStrata(ctx, strata)
	if estimate == 0 {
		// Send code 304 NOT MODIFIED
		err = rsph.SendCode(304)
		if err != nil {
			return fmt.Errorf("error sending 304 not modified: %v", err)
		}
		log.Debugf("%s nodes are synced: %v", gn.ID(), err)
		return nil
	}

	var remoteWants *WantMessage

	if diff == nil {
		// step 3 - send appropriate IBF
		err = rsph.SendBloomFilter(ctx, estimate)
		if err != nil {
			return fmt.Errorf("error sending bloom filter: %v", err)
		}

		// step 4 - wait for wants
		remoteWants, err = rsph.WaitForWantsMessage(ctx)
		if err != nil {
			return fmt.Errorf("error waiting on wants message: %v", err)
		}
		// if the wants didn't come through and there wasn't an error
		// then the invertible filter just failed to decode, and we can move on
		if remoteWants == nil {
			return nil
		}
	} else {
		// Send code 302 FOUND to indicate we can skip bloom filter exchange
		err = rsph.SendCode(302)
		if err != nil {
			return fmt.Errorf("error sending 302 FOUND: %v", err)
		}
		log.Debugf("%s sending code 302 to %s", gn.ID(), rsph.peerID)
		remoteWants = WantMessageFromDiff(diff.LeftSet)

		myWants := WantMessageFromDiff(diff.RightSet)
		err = rsph.SendWantMessage(ctx, myWants)
		if err != nil {
			return fmt.Errorf("error sending my wants: %v", err)
		}
	}

	// step 5 - handle incoming obects and send wanted objects

	wg := &sync.WaitGroup{}
	results := make(chan error, 2)

	wg.Add(1)
	go func() {
		// Step 4: handle incoming objects
		results <- rsph.WaitForProvides(ctx)
		wg.Done()
	}()

	wg.Add(1)
	go func() {
		results <- rsph.SendWantedObjects(ctx, remoteWants)
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

func (rsph *ReceiveSyncProtocolHandler) SendWantMessage(ctx context.Context, want *WantMessage) error {
	writer := rsph.writer
	err := want.EncodeMsg(writer)
	if err != nil {
		return fmt.Errorf("error writing wants: %v", err)
	}

	return writer.Flush()
}

func (rsph *ReceiveSyncProtocolHandler) SendCode(code int) error {
	writer := rsph.writer
	pm := &ProtocolMessage{
		Code: code,
	}
	return pm.EncodeMsg(writer)
}

func (rsph *ReceiveSyncProtocolHandler) Send503IfTooManyInProgress(ctx context.Context) (*SyncHandlerWorker, error) {
	gn := rsph.gossipNode
	select {
	case worker := <-gn.syncPool:
		return &worker, nil
	default:
		writer := rsph.writer
		pm := &ProtocolMessage{
			Code: 503,
		}
		err := pm.EncodeMsg(writer)
		if err != nil {
			log.Errorf("%s error writing wants: %v", gn.ID(), err)
			return nil, fmt.Errorf("error writing wants: %v", err)
		}
		return nil, nil
	}
}

func (rsph *ReceiveSyncProtocolHandler) WaitForWantsMessage(ctx context.Context) (*WantMessage, error) {
	reader := rsph.reader
	gn := rsph.gossipNode
	var pm ProtocolMessage
	err := pm.DecodeMsg(reader)
	if err != nil {
		log.Errorf("%s error reading protocol message %v", gn.ID(), err)
		return nil, fmt.Errorf("error decoding protocol message: %v", err)
	}

	switch pm.Code {
	case 416:
		// 416 REQUESTED RANGE NOT SATISFIABLE
		// just means the bloom filter didn't decode, which is expected behavior given changes
		// on remote side during this protocol exchange
		log.Infof("%s error decoding IBF on peer %s: %d, %v", gn.ID(), rsph.peerID, pm.Code, pm.Error)
		return nil, nil
	case 500:
		log.Errorf("%s error from remote peer %s: %d, %v", gn.ID(), rsph.peerID, pm.Code, pm.Error)
		return nil, fmt.Errorf("remote error: %v", err)
	}

	wants, err := FromProtocolMessage(&pm)
	if err != nil {
		return nil, fmt.Errorf("error converting wants: %v", err)
	}
	return wants.(*WantMessage), nil
}

func (rsph *ReceiveSyncProtocolHandler) SendBloomFilter(ctx context.Context, estimate int) error {
	gn := rsph.gossipNode
	writer := rsph.writer

	span, _ := newSpan(ctx, gn.Tracer, "SendBloomFilter")
	defer span.Finish()

	wantsToSend := estimate * 2
	var sizeToSend int

	for _, size := range standardIBFSizes {
		if size >= wantsToSend {
			sizeToSend = size
			break
		}
	}
	if sizeToSend == 0 {
		log.Errorf("%s estimate too large to send an IBF: %d", gn.ID(), estimate)
		return fmt.Errorf("error estimate is too large: %d", estimate)
	}

	log.Debugf("%s sending bloom filter to %s of size: %d", gn.ID(), rsph.peerID, sizeToSend)
	gn.ibfSyncer.RLock()
	pm, err := ToProtocolMessage(gn.IBFs[sizeToSend])
	gn.ibfSyncer.RUnlock()
	if err != nil {
		return fmt.Errorf("error turning IBF into protocol message: %v", err)
	}

	err = pm.EncodeMsg(writer)
	if err != nil {
		return fmt.Errorf("error writing IBF: %v", err)
	}
	log.Debugf("%s flushing", gn.ID())
	return writer.Flush()
}

func (rsph *ReceiveSyncProtocolHandler) WaitForProvides(ctx context.Context) error {
	gn := rsph.gossipNode
	reader := rsph.reader
	var isLastMessage bool
	span, funcCtx := newSpan(ctx, gn.Tracer, "WaitForProvides")
	defer span.Finish()

	for !isLastMessage {
		msgSpan, msgCtx := newSpan(funcCtx, gn.Tracer, "HandleMessage")
		var provideMsg ProvideMessage
		err := provideMsg.DecodeMsg(reader)
		if err != nil {
			log.Errorf("%s error decoding message: %v", gn.ID(), err)
			return fmt.Errorf("error decoding message: %v", err)
		}
		log.Debugf("%s HandleSync new msg from %s %s %v", gn.ID(), rsph.peerID, bytesToString(provideMsg.Key), provideMsg.Key)
		isLastMessage = provideMsg.Last
		if !isLastMessage {
			provideMsg.From = rsph.peerID
			queueSpan, _ := newSpan(msgCtx, gn.Tracer, "QueueMessage", opentracing.Tag{Key: "span.kind", Value: "producer"})
			provideMsg.spanContext = queueSpan.Context()
			gn.newObjCh <- provideMsg
			queueSpan.Finish()
		}
		msgSpan.Finish()
	}

	log.Debugf("%s: HandleSync received all provides from %s, moving forward", gn.ID(), rsph.peerID)
	return nil
}

func (rsph *ReceiveSyncProtocolHandler) WaitForStrata(ctx context.Context) (*ibf.DifferenceStrata, error) {
	reader := rsph.reader
	gn := rsph.gossipNode

	span, _ := newSpan(ctx, gn.Tracer, "WaitForStrata")
	defer span.Finish()

	var remoteStrata ibf.DifferenceStrata
	err := remoteStrata.DecodeMsg(reader)
	if err != nil {
		log.Errorf("%s error decoding message", gn.ID(), err)
		return &remoteStrata, fmt.Errorf("error decoding message: %v", err)
	}
	return &remoteStrata, nil
}

func (rsph *ReceiveSyncProtocolHandler) EstimateFromRemoteStrata(ctx context.Context, remoteStrata *ibf.DifferenceStrata) (count int, result *ibf.DecodeResults) {
	// log.Debugf("%s from %s received IBF (cells)", gn.ID(), peerID, ibf.HumanizeIBF(&remoteIBF))
	gn := rsph.gossipNode

	span, _ := newSpan(ctx, gn.Tracer, "EstimateFromRemoteStrata")
	defer span.Finish()

	gn.ibfSyncer.RLock()
	count, result = gn.Strata.Estimate(remoteStrata)
	gn.ibfSyncer.RUnlock()
	log.Debugf("%s estimated difference from %s is %d", gn.ID(), rsph.peerID, count)
	return count, result
}

func (rsph *ReceiveSyncProtocolHandler) SendWantedObjects(ctx context.Context, wants *WantMessage) error {
	gn := rsph.gossipNode
	writer := rsph.writer

	span, _ := newSpan(ctx, gn.Tracer, "SendWantedObjects")
	defer span.Finish()

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
