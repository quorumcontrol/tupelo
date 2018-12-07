package gossip2

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	logging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/quorumcontrol/differencedigest/ibf"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/bls"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/quorumcontrol/tupelo/p2p"
)

func init() {
	// SEE: https://github.com/quorumcontrol/storage
	// Using badger suggests a minimum of 128 GOMAXPROCS, but also
	// allow the user to customize
	if os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(128)
	}
}

var log = logging.Logger("gossip")

var objectPrefix = []byte("O-")

const syncProtocol = "tupelo-gossip/v1"
const IsDoneProtocol = "tupelo-done/v1"
const TipProtocol = "tupelo-tip/v1"
const NewTransactionProtocol = "tupelo-new-transaction/v1"
const NewSignatureProtocol = "tupelo-new-signature/v1"
const ChainTreeChangeProtocol = "tupelo-chain-change/v1"

const minSyncNodesPerTransaction = 3
const workerCount = 100

const useDoneRemovalFilter = true

type IBFMap map[int]*ibf.InvertibleBloomFilter

var standardIBFSizes = []int{500, 2000, 100000}

type SyncHandlerWorker struct{}

type StateTransaction struct {
	ObjectID     []byte
	Transaction  []byte
	CurrentState []byte
}

// StateHandler is the function that takes a stateTransition object and returns the nextState, whether it's accepted or not or an error
type StateHandler func(ctx context.Context, stateTrans StateTransaction) (nextState []byte, accepted bool, err error)

type GossipNode struct {
	Key              *ecdsa.PrivateKey
	SignKey          *bls.SignKey
	verKey           *bls.VerKey
	address          string
	id               string
	Host             p2p.Node
	Storage          storage.Storage
	Strata           *ibf.DifferenceStrata
	Group            *consensus.NotaryGroup
	IBFs             IBFMap
	GossipReporter   GossipReporter
	Tracer           opentracing.Tracer
	newObjCh         chan ProvideMessage
	stopChan         chan struct{}
	ibfSyncer        *sync.RWMutex
	syncPool         chan SyncHandlerWorker
	sigSendingCh     chan envelope
	debugReceiveSync uint64
	debugSendSync    uint64
	debugAttemptSync uint64
	subscriptions    *subscriptionHolder
}

const NumberOfSyncWorkers = 3

func NewGossipNode(dstKey *ecdsa.PrivateKey, signKey *bls.SignKey, host p2p.Node, storage storage.Storage) *GossipNode {
	node := &GossipNode{
		Key:            dstKey,
		SignKey:        signKey,
		Host:           host,
		Storage:        storage,
		Strata:         ibf.NewDifferenceStrata(),
		IBFs:           make(IBFMap),
		GossipReporter: new(NoOpReporter),
		//TODO: examine the 5 here?
		newObjCh:      make(chan ProvideMessage, 5),
		stopChan:      make(chan struct{}, 3),
		ibfSyncer:     &sync.RWMutex{},
		syncPool:      make(chan SyncHandlerWorker, NumberOfSyncWorkers),
		sigSendingCh:  make(chan envelope, workerCount+1),
		subscriptions: newSubscriptionHolder(),
		Tracer:        opentracing.GlobalTracer(),
	}
	node.verKey = node.SignKey.MustVerKey()
	node.address = consensus.BlsVerKeyToAddress(node.verKey.Bytes()).String()
	for i := 0; i < NumberOfSyncWorkers; i++ {
		node.syncPool <- SyncHandlerWorker{}
	}

	go node.handleNewObjCh()
	go node.handleSigSendingCh()

	for _, size := range standardIBFSizes {
		node.IBFs[size] = ibf.NewInvertibleBloomFilter(size, 4)
		// node.IBFs[size].TurnOnDebug()
	}

	storage.ForEachKey([]byte{}, func(key []byte) error {
		node.addKeyToIBFs(key)
		return nil
	})

	host.SetStreamHandler(syncProtocol, node.HandleSync)
	host.SetStreamHandler(IsDoneProtocol, node.HandleDoneProtocol)
	host.SetStreamHandler(TipProtocol, node.HandleTipProtocol)
	host.SetStreamHandler(NewTransactionProtocol, node.HandleNewTransactionProtocol)
	host.SetStreamHandler(NewSignatureProtocol, node.HandleNewSignatureProtocol)
	host.SetStreamHandler(ChainTreeChangeProtocol, node.HandleChainTreeChangeProtocol)
	return node
}

func (gn *GossipNode) Start() {
	pauser := make(chan time.Time, 1)
	time.AfterFunc(100*time.Millisecond, func() {
		pauser <- time.Now()
	})
	for {
		select {
		case <-gn.stopChan:
			return
		case <-pauser:
			err := gn.DoSync()
			timeToSleep := 100 * time.Millisecond
			if err != nil {
				if err == p2p.ErrDialBackoff {
					// slow things down if we got an errDialBackoff
					timeToSleep = 500 * time.Millisecond
				} else {
					log.Errorf("%s error doing sync %v", gn.ID(), err)
				}
			}
			time.AfterFunc(timeToSleep, func() {
				pauser <- time.Now()
			})
		}
	}
}

func (gn *GossipNode) Stop() {
	gn.stopChan <- struct{}{}
	gn.stopChan <- struct{}{}
	gn.stopChan <- struct{}{}
}

type conflictSetStats struct {
	SignatureCount  uint32
	IsDone          bool
	HasTransaction  bool
	InProgress      bool
	PendingMessages []ProvideMessage
}

type processorResponse struct {
	ConflictSetID  []byte
	IsDone         bool
	NewSignature   bool
	NewTransaction bool
	Error          error
	spanContext    opentracing.SpanContext
}

type handlerRequest struct {
	responseChan  chan processorResponse
	msg           ProvideMessage
	conflictSetID []byte
}

func (gn *GossipNode) handleToProcessChan(ch chan handlerRequest) {
	for {
		select {
		case req := <-ch:
			(&conflictSetWorker{gn: gn}).HandleRequest(req.msg, req.responseChan)
			gn.GossipReporter.Processed(gn.ID(), req.msg.From, req.msg.Key, time.Now())
		case <-gn.stopChan:
			return
		}
	}
}

func (gn *GossipNode) handleNewObjCh() {
	inProgressConflictSets := make(map[string]*conflictSetStats)
	responseChan := make(chan processorResponse, workerCount+1)
	toProcessChan := make(chan handlerRequest, workerCount)
	for i := 0; i < workerCount; i++ {
		go gn.handleToProcessChan(toProcessChan)
	}
	for {
		select {
		case msg := <-gn.newObjCh:
			log.Debugf("%s new object chan length: %d", gn.ID(), len(gn.newObjCh))
			opts := []opentracing.StartSpanOption{opentracing.Tag{Key: "span.kind", Value: "consumer"}}
			if spanContext := msg.spanContext; spanContext != nil {
				opts = append(opts, opentracing.FollowsFrom(spanContext))
			}
			span, ctx := newSpan(context.Background(), gn.Tracer, "HandleNewObjectChannel", opts...)

			conflictSetID := conflictSetIDFromMessageKey(msg.Key)
			conflictSetStat, ok := inProgressConflictSets[string(conflictSetID)]
			if !ok {
				conflictSetStat = new(conflictSetStats)
				inProgressConflictSets[string(conflictSetID)] = conflictSetStat
			}

			if conflictSetStat.IsDone {
				// we can just stop here
				log.Debugf("%s ignore message because conflict set is done %v", gn.ID(), msg.Key)
				span.Finish()
				continue
			}

			// If another worker is processing this conflict set, queue up this message for later processing
			if conflictSetStat.InProgress {
				conflictSetStat.PendingMessages = append(conflictSetStat.PendingMessages, msg)
				span.Finish()
				continue
			}

			didSet, err := gn.Add(msg.Key, msg.Value)

			if err != nil {
				log.Errorf("%s error adding: %v", gn.ID(), err)
			}

			// This message already existed, skip further processing
			if !didSet {
				gn.popPendingMessage(ctx, conflictSetStat)
				span.Finish()
				continue
			}
			gn.GossipReporter.Seen(gn.ID(), msg.From, msg.Key, time.Now())

			messageType := msg.Type()

			if messageType == MessageTypeTransaction && conflictSetStat.HasTransaction {
				gn.popPendingMessage(ctx, conflictSetStat)
				continue
			}

			conflictSetStat.InProgress = true
			queueSpan, _ := newSpan(ctx, gn.Tracer, "QueueForProcessing", opentracing.Tag{Key: "span.kind", Value: "producer"})
			msg.spanContext = queueSpan.Context()
			toProcessChan <- handlerRequest{
				responseChan:  responseChan,
				msg:           msg,
				conflictSetID: conflictSetID,
			}
			queueSpan.Finish()
			span.Finish()
		case resp := <-responseChan:
			opts := []opentracing.StartSpanOption{opentracing.Tag{Key: "span.kind", Value: "consumer"}}
			if spanContext := resp.spanContext; spanContext != nil {
				opts = append(opts, opentracing.FollowsFrom(spanContext))
			}
			span, ctx := newSpan(context.Background(), gn.Tracer, "HandlingResponse", opts...)

			conflictSetStat, ok := inProgressConflictSets[string(resp.ConflictSetID)]
			if !ok {
				panic("this should never happen")
			}
			if conflictSetStat.IsDone {
				span.Finish()
				continue
			}
			if resp.Error != nil {
				// at minimum delete it out of storage
				panic(fmt.Sprintf("TODO: handle this gracefully: %v", resp.Error))
			}
			if resp.NewTransaction {
				conflictSetStat.HasTransaction = true
			}

			if resp.NewSignature {
				conflictSetStat.SignatureCount++
				log.Debugf("%s sig count: %d, conflict set id %v", gn.ID(), conflictSetStat.SignatureCount, resp.ConflictSetID)
			}

			if resp.IsDone {
				conflictSetStat.IsDone = true
				conflictSetStat.PendingMessages = make([]ProvideMessage, 0)
				gn.popPendingMessage(ctx, conflictSetStat)
				conflictSetStat.InProgress = false
			} else {
				roundInfo, err := gn.Group.MostRecentRoundInfo(gn.Group.RoundAt(time.Now()))
				if err != nil {
					log.Errorf("%s could not fetch roundinfo %v", gn.ID(), err)
				}

				if roundInfo != nil && int64(conflictSetStat.SignatureCount) >= roundInfo.SuperMajorityCount() {
					go gn.signatureCheckWorker(ctx, resp.ConflictSetID, conflictSetStat)
				} else {
					gn.popPendingMessage(ctx, conflictSetStat)
					conflictSetStat.InProgress = false
				}
			}
			span.Finish()

		case <-gn.stopChan:
			return
		}
	}
}

func (gn *GossipNode) signatureCheckWorker(ctx context.Context, conflictSetID []byte, conflictSetStat *conflictSetStats) {
	log.Debugf("%s reached super majority", gn.ID())
	provideMessage, err := gn.checkSignatures(conflictSetID, conflictSetStat)

	if err != nil {
		log.Errorf("%s error for checkSignatureCounts: %v", gn.ID(), err)
	}

	if provideMessage != nil {
		log.Debugf("%s queueing up %v message", gn.ID(), provideMessage.Type().string())
		conflictSetStat.PendingMessages = append([]ProvideMessage{*provideMessage}, conflictSetStat.PendingMessages...)
	}

	gn.popPendingMessage(ctx, conflictSetStat)
	conflictSetStat.InProgress = false
}

func (gn *GossipNode) popPendingMessage(ctx context.Context, conflictSetStat *conflictSetStats) bool {
	span, ctx := newSpan(ctx, gn.Tracer, "popPendingMessage")

	if len(conflictSetStat.PendingMessages) > 0 {
		log.Debugf("%s pending messages, queuing one", gn.ID())
		nextMessage, remainingMessages := conflictSetStat.PendingMessages[0], conflictSetStat.PendingMessages[1:]
		conflictSetStat.PendingMessages = remainingMessages
		go func(msg ProvideMessage) {
			log.Debugf("%s queuing popped message: %v", gn.ID(), msg.Key)
			gn.newObjCh <- msg
			span.Finish()
		}(nextMessage)
		return true
	}
	span.Finish()
	return false
}

type envelope struct {
	Destination ecdsa.PublicKey
	Msg         ProvideMessage
}

func (gn *GossipNode) handleSigSendingCh() {
	for env := range gn.sigSendingCh {
		bits, err := env.Msg.MarshalMsg(nil)
		if err != nil {
			log.Errorf("error marshaling message: %v", err)
		}
		gn.Host.Send(&env.Destination, NewSignatureProtocol, bits)
	}
}

func (gn *GossipNode) InitiateTransaction(t Transaction) ([]byte, error) {
	encodedTrans, err := t.MarshalMsg(nil)
	if err != nil {
		return nil, fmt.Errorf("error encoding: %v", err)
	}
	log.Debugf("%s initiating transaction %v", gn.ID(), t.StoredID())
	gn.newObjCh <- ProvideMessage{Key: t.StoredID(), Value: encodedTrans, From: gn.ID()}
	return t.ID(), nil
}

func (gn *GossipNode) ID() string {
	if gn.id != "" {
		return gn.id
	}
	gn.id = gn.Host.Identity()
	return gn.id
}

func (gn *GossipNode) isMessageDone(msg ProvideMessage) (bool, error) {
	return isDone(gn.Storage, msg.Key[len(msg.Key)-32:])
}

func (gn *GossipNode) checkSignatures(conflictSetID []byte, conflictSetStat *conflictSetStats) (*ProvideMessage, error) {
	conflictSetKeys, err := gn.Storage.GetKeysByPrefix(conflictSetID[0:4])
	if err != nil {
		return nil, fmt.Errorf("error fetching keys %v", err)
	}
	var matchedSigKeys [][]byte

	for _, k := range conflictSetKeys {
		if messageTypeFromKey(k) == MessageTypeSignature && bytes.Equal(conflictSetIDFromMessageKey(k), conflictSetID) {
			matchedSigKeys = append(matchedSigKeys, k)
		}
	}

	roundInfo, err := gn.Group.MostRecentRoundInfo(gn.Group.RoundAt(time.Now()))
	if err != nil {
		return nil, fmt.Errorf("error fetching roundinfo %v", err)
	}
	superMajorityCount := roundInfo.SuperMajorityCount()

	if int64(len(matchedSigKeys)) < superMajorityCount {
		return nil, nil
	}

	rawSignatures, err := gn.Storage.GetAll(matchedSigKeys)
	if err != nil {
		return nil, fmt.Errorf("error fetching signers %v", err)
	}

	storedSignatures := make(map[string]Signature)

	for _, rawSig := range rawSignatures {
		var s Signature
		_, err = s.UnmarshalMsg(rawSig.Value)
		if err != nil {
			return nil, fmt.Errorf("error unmarshaling sig: %v", err)
		}
		storedSignatures[string(rawSig.Key)] = s
	}

	signersByTransactionID := make(map[string]map[string]Signature)
	for _, signature := range storedSignatures {
		transID := string(signature.TransactionID)

		for i, didSign := range signature.Signers {
			if didSign {
				k := roundInfo.Signers[i].Id
				if _, ok := signersByTransactionID[transID]; !ok {
					signersByTransactionID[transID] = make(map[string]Signature)
				}
				signersByTransactionID[transID][k] = signature
			}
		}
	}

	var majorityTransactionID []byte
	var majoritySigners map[string]Signature
	var totalNumberOfSignatures int

	for transactionID, signers := range signersByTransactionID {
		totalNumberOfSignatures = totalNumberOfSignatures + len(signers)
		if int64(len(signers)) >= superMajorityCount {
			majorityTransactionID = []byte(transactionID)
			majoritySigners = signers
			break
		}
	}

	if len(majoritySigners) == 0 {
		// Deadlock detection
		numberOfRemainingSigners := len(roundInfo.Signers) - totalNumberOfSignatures
		foundSignableTransaction := false

		for _, signers := range signersByTransactionID {
			if int64(len(signers)+numberOfRemainingSigners) >= superMajorityCount {
				foundSignableTransaction = true
			}
		}

		// Its deadlocked
		// For now, given the lack of slashing, just
		// remove conflicting transactions
		if !foundSignableTransaction {
			var lowestTransactionID string
			for transactionID, _ := range signersByTransactionID {
				if lowestTransactionID == "" || transactionID < lowestTransactionID {
					lowestTransactionID = transactionID
				}
			}
			lowestTransactionStoredID := storedIDForTransactionIDAndConflictSetID([]byte(lowestTransactionID), conflictSetID)
			log.Debugf("%s detected deadlock for conflict set %v, selecting transaction %v", gn.ID(), conflictSetID, lowestTransactionStoredID)

			hasSigned := false
			conflictSetStat.SignatureCount = 0

			for storedKey, signature := range storedSignatures {
				if lowestTransactionID == string(signature.TransactionID) {
					conflictSetStat.SignatureCount = conflictSetStat.SignatureCount + uint32(len(signature.Signers))
					idx := indexOfAddr(roundInfo.Signers, gn.address)
					hasSigned = signature.Signers[idx]
				} else {
					gn.Remove([]byte(storedKey))
				}
			}

			conflictSetStat.HasTransaction = false
			for _, k := range conflictSetKeys {
				if messageTypeFromKey(k) == MessageTypeTransaction && bytes.Equal(conflictSetIDFromMessageKey(k), conflictSetID) {
					if bytes.Equal(k, lowestTransactionStoredID) {
						conflictSetStat.HasTransaction = true
					} else {
						gn.Remove(k)
					}
				}
			}

			if !hasSigned {
				transactionBytes, err := gn.Storage.Get(lowestTransactionStoredID)
				if err != nil {
					return nil, fmt.Errorf("error fetching trans: %v", err)
				}

				if len(transactionBytes) > 0 {
					var lowestTrans Transaction
					_, err := lowestTrans.UnmarshalMsg(transactionBytes)
					if err != nil {
						return nil, fmt.Errorf("error unmarshaling lowest trans: %v", err)
					}
					sig, err := lowestTrans.Sign(gn.SignKey)
					if err != nil {
						return nil, fmt.Errorf("error signing key: %v", err)
					}
					signers := make([]bool, len(roundInfo.Signers))
					signers[indexOfAddr(roundInfo.Signers, gn.address)] = true
					signature := Signature{
						TransactionID: []byte(lowestTransactionID),
						ObjectID:      lowestTrans.ObjectID,
						Tip:           lowestTrans.NewTip,
						Signers:       signers,
						Signature:     sig,
					}
					encodedSig, err := signature.MarshalMsg(nil)

					log.Debugf("%s add signature to remove deadlock %v", gn.ID(), signature.StoredID(conflictSetID))

					return &ProvideMessage{
						Key:   signature.StoredID(conflictSetID),
						Value: encodedSig,
					}, nil
				} else {
					log.Debugf("%s attempted to add signature to remove deadlock, but transaction does not exist yet %v", gn.ID(), lowestTransactionStoredID)
				}
			} else {
				log.Debugf("%s has already signed deadlocked transaction %v", gn.ID(), lowestTransactionStoredID)
			}
		}

		return nil, nil
	}

	allSigners := make([]bool, len(roundInfo.Signers))
	var signatureBytes [][]byte
	var majorityObjectID []byte
	var majorityNewTip []byte
	for i, signer := range roundInfo.Signers {
		key := consensus.BlsVerKeyToAddress(signer.VerKey.PublicKey).String()

		if sig, ok := majoritySigners[key]; ok {
			signatureBytes = append(signatureBytes, sig.Signature)
			majorityObjectID = sig.ObjectID
			majorityNewTip = sig.Tip
			allSigners[i] = true
		}
	}

	combinedSignatures, err := bls.SumSignatures(signatureBytes)
	if err != nil {
		return nil, fmt.Errorf("error combining sigs %v", err)
	}

	commitSignature := Signature{
		TransactionID: majorityTransactionID,
		Signers:       allSigners,
		Signature:     combinedSignatures,
	}
	currentState := CurrentState{
		Tip:       majorityNewTip,
		ObjectID:  majorityObjectID,
		Signature: commitSignature,
	}

	encodedState, err := currentState.MarshalMsg(nil)
	if err != nil {
		return nil, fmt.Errorf("error marshaling sig: %v", err)
	}
	doneID := doneIDFromConflictSetID(conflictSetID)

	log.Debugf("%s adding done state %v", gn.ID(), doneID)
	doneMessage := ProvideMessage{
		Key:   doneID,
		Value: encodedState,
	}

	return &doneMessage, nil
}

func (gn *GossipNode) Add(key, value []byte) (bool, error) {
	didSet, err := gn.Storage.SetIfNotExists(key, value)
	if err != nil {
		log.Errorf("%s error setting: %v", gn.ID(), err)
		return false, fmt.Errorf("error setting storage: %v", err)
	}
	if didSet {
		gn.addKeyToIBFs(key)
	} else {
		log.Debugf("%s skipped adding, already exists %v", gn.ID(), key)
	}
	return didSet, nil
}

func (gn *GossipNode) Remove(key []byte) {
	ibfObjectID := byteToIBFsObjectId(key[0:8])
	err := gn.Storage.Delete(key)
	if err != nil {
		panic("storage failed")
	}
	gn.ibfSyncer.Lock()
	gn.Strata.Remove(ibfObjectID)
	for _, filter := range gn.IBFs {
		filter.Remove(ibfObjectID)
	}
	gn.ibfSyncer.Unlock()
	log.Debugf("%s removed key %v", gn.ID(), key)
}

func (gn *GossipNode) randomPeer() (*consensus.RemoteNode, error) {
	roundInfo, err := gn.Group.MostRecentRoundInfo(gn.Group.RoundAt(time.Now()))
	if err != nil {
		return nil, fmt.Errorf("error getting peer: %v", err)
	}
	var peer *consensus.RemoteNode
	for peer == nil {
		member := roundInfo.RandomMember()
		if !bytes.Equal(member.DstKey.PublicKey, crypto.FromECDSAPub(&gn.Key.PublicKey)) {
			peer = member
		}
	}
	return peer, nil
}

func (gn *GossipNode) getSyncTarget() (*consensus.RemoteNode, error) {
	return gn.randomPeer()
}

func (gn *GossipNode) DoSync() error {
	return DoSyncProtocol(gn)
}

func (gn *GossipNode) HandleNewSignatureProtocol(stream net.Stream) {
	err := DoNewSignatureProtocol(gn, stream)
	if err != nil {
		log.Errorf("%s error handling new sig: %v", gn.ID(), err)
	}
}

func (gn *GossipNode) HandleSync(stream net.Stream) {
	err := DoReceiveSyncProtocol(gn, stream)
	if err != nil {
		log.Errorf("error handling sync: %v", err)
	}
}

func (gn *GossipNode) HandleDoneProtocol(stream net.Stream) {
	err := DoDoneProtocol(gn, stream)
	if err != nil {
		log.Errorf("error handling done protocol: %v", err)
	}
}

func (gn *GossipNode) HandleTipProtocol(stream net.Stream) {
	err := DoTipProtocol(gn, stream)
	if err != nil {
		log.Errorf("error handling tip protocol: %v", err)
	}
}

func (gn *GossipNode) HandleNewTransactionProtocol(stream net.Stream) {
	err := DoNewTransactionProtocol(gn, stream)
	if err != nil {
		log.Errorf("error handling new transaction protocol: %v", err)
	}
}

func (gn *GossipNode) HandleChainTreeChangeProtocol(stream net.Stream) {
	err := DoChainTreeChangeProtocol(gn, stream)
	if err != nil {
		log.Errorf("error handling chaintree change protocol: %v", err)
	}
}

func byteToIBFsObjectId(byteID []byte) ibf.ObjectId {
	if len(byteID) != 8 {
		panic("invalid byte id sent")
	}
	return ibf.ObjectId(bytesToUint64(byteID))
}

func bytesToUint64(byteID []byte) uint64 {
	return binary.BigEndian.Uint64(byteID)
}

func uint64ToBytes(id uint64) []byte {
	a := make([]byte, 8)
	binary.BigEndian.PutUint64(a, id)
	return a
}

func concatBytesSlice(byteSets ...[]byte) (concat []byte) {
	for _, s := range byteSets {
		concat = append(concat, s...)
	}
	return concat
}

func bytesToString(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}
func mustStringToBytes(str string) []byte {
	data, err := base64.StdEncoding.DecodeString(str)
	if err != nil {
		panic(fmt.Sprintf("error: %v", err))
	}
	return data
}

func (gn *GossipNode) syncTargetsByRoutingKey(key []byte) ([]*consensus.RemoteNode, error) {
	roundInfo, err := gn.Group.MostRecentRoundInfo(gn.Group.RoundAt(time.Now()))
	if err != nil {
		return nil, fmt.Errorf("error fetching roundinfo %v", err)
	}

	signerCount := float64(len(roundInfo.Signers))
	logOfSigners := math.Log(signerCount)
	numberOfTargets := math.Floor(math.Max(logOfSigners, float64(minSyncNodesPerTransaction)))
	indexSpacing := signerCount / numberOfTargets
	moduloOffset := math.Mod(float64(bytesToUint64(key)), indexSpacing)

	targets := make([]*consensus.RemoteNode, int(numberOfTargets))

	for i := 0; i < int(numberOfTargets); i++ {
		targetIndex := int64(math.Floor(moduloOffset + (indexSpacing * float64(i))))
		target := roundInfo.Signers[targetIndex]

		// Make sure this node doesn't add itself as a target
		if bytes.Equal(target.DstKey.PublicKey, crypto.FromECDSAPub(&gn.Key.PublicKey)) {
			continue
		}
		targets[i] = target

	}
	return targets, nil
}

func indexOfAddr(signers []*consensus.RemoteNode, addr string) int {
	for i, s := range signers {
		if s.Id == addr {
			return i
		}
	}
	return -1
}

func (gn *GossipNode) addKeyToIBFs(key []byte) {
	msgType := messageTypeFromKey(key)
	// This allows only Transaction / Done to be synced via IBF
	switch msgType {
	case MessageTypeTransaction:
	case MessageTypeDone:
	default:
		return
	}

	log.Debugf("%s adding %v", gn.ID(), key)
	ibfObjectID := byteToIBFsObjectId(key[0:8])
	gn.ibfSyncer.Lock()
	gn.Strata.Add(ibfObjectID)
	for _, filter := range gn.IBFs {
		filter.Add(ibfObjectID)
	}
	gn.ibfSyncer.Unlock()
}

func (gn *GossipNode) getCurrentState(objID []byte) (*CurrentState, error) {
	stateBytes, err := gn.Storage.Get(objectIdWithPrefix(objID))
	if err != nil {
		return nil, fmt.Errorf("error getting bytes: %v", err)
	}
	if len(stateBytes) > 0 {
		var state CurrentState
		_, err := state.UnmarshalMsg(stateBytes)
		if err != nil {
			return nil, fmt.Errorf("error unmarshaling: %v", err)
		}
		return &state, nil
	}
	return nil, nil
}

func objectIdWithPrefix(objId []byte) []byte {
	return append(objectPrefix, objId...)
}
