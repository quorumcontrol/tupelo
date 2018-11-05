package gossip2

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	logging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/quorumcontrol/differencedigest/ibf"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/p2p"
)

var log = logging.Logger("gossip")

const syncProtocol = "tupelo-gossip/v1"
const IsDoneProtocol = "tupelo-done/v1"
const NewTransactionProtocol = "tupelo-new-transaction/v1"
const NewSignatureProtocol = "tupelo-new-signature/v1"

const minSyncNodesPerTransaction = 3
const workerCount = 100

const useDoneRemovalFilter = true

type IBFMap map[int]*ibf.InvertibleBloomFilter

var standardIBFSizes = []int{500, 2000, 100000}

type MessageType int

const (
	MessageTypeSignature MessageType = iota + 1
	MessageTypeTransaction
	MessageTypeDone
)

type SyncHandlerWorker struct{}

type GossipNode struct {
	Key              *ecdsa.PrivateKey
	SignKey          *bls.SignKey
	verKey           *bls.VerKey
	address          string
	id               string
	Host             *p2p.Host
	Storage          *BadgerStorage
	Strata           *ibf.DifferenceStrata
	Group            *consensus.NotaryGroup
	IBFs             IBFMap
	newObjCh         chan ProvideMessage
	stopChan         chan struct{}
	ibfSyncer        *sync.RWMutex
	syncPool         chan SyncHandlerWorker
	sigSendingCh     chan envelope
	debugReceiveSync uint64
	debugSendSync    uint64
	debugAttemptSync uint64
}

const NumberOfSyncWorkers = 3

func NewGossipNode(dstKey *ecdsa.PrivateKey, signKey *bls.SignKey, host *p2p.Host, storage *BadgerStorage) *GossipNode {
	node := &GossipNode{
		Key:     dstKey,
		SignKey: signKey,
		Host:    host,
		Storage: storage,
		Strata:  ibf.NewDifferenceStrata(),
		IBFs:    make(IBFMap),
		//TODO: examine the 5 here?
		newObjCh:     make(chan ProvideMessage, 5),
		stopChan:     make(chan struct{}, 3),
		ibfSyncer:    &sync.RWMutex{},
		syncPool:     make(chan SyncHandlerWorker, NumberOfSyncWorkers),
		sigSendingCh: make(chan envelope, workerCount+1),
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
	host.SetStreamHandler(syncProtocol, node.HandleSync)
	host.SetStreamHandler(IsDoneProtocol, node.HandleDoneProtocol)
	host.SetStreamHandler(NewTransactionProtocol, node.HandleNewTransactionProtocol)
	host.SetStreamHandler(NewSignatureProtocol, node.HandleNewSignatureProtocol)
	return node
}

func (gn *GossipNode) Start() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-gn.stopChan:
			return
		case <-ticker.C:
			err := gn.DoSync()
			if err != nil {
				log.Errorf("%s error doing sync %v", gn.ID(), err)
			}

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
			conflictSetID := conflictSetIDFromMessageKey(msg.Key)
			conflictSetStat, ok := inProgressConflictSets[string(conflictSetID)]
			if !ok {
				conflictSetStat = new(conflictSetStats)
				inProgressConflictSets[string(conflictSetID)] = conflictSetStat
			}
			if conflictSetStat.IsDone {
				// we can just stop here
				continue
			}
			didSet, err := gn.Add(msg.Key, msg.Value)
			if err != nil {
				log.Errorf("%s error adding: %v", gn.ID(), err)
			}
			if didSet {
				log.Debugf("%s did set", gn.ID())
				if conflictSetStat.InProgress {
					// messageType := MessageType(msg.Key[8])
					// switch messageType {
					// case MessageTypeTransaction, MessageTypeDone:
					// 	conflictSetStat.PendingMessages = append([]ProvideMessage{msg}, conflictSetStat.PendingMessages...)
					// default:
					conflictSetStat.PendingMessages = append(conflictSetStat.PendingMessages, msg)
					// }
				} else {
					conflictSetStat.InProgress = true
					log.Debugf("%s sending to process chan: %s", gn.ID(), msg.Key)
					toProcessChan <- handlerRequest{
						responseChan:  responseChan,
						msg:           msg,
						conflictSetID: conflictSetID,
					}

				}
			}

		case resp := <-responseChan:
			conflictSetStat, ok := inProgressConflictSets[string(resp.ConflictSetID)]
			conflictSetStat.InProgress = false
			if !ok {
				panic("this should never happen")
			}
			if resp.Error != nil {
				// at minimum delete it out of storage
				panic("TODO: handle this gracefully")
			}
			if resp.NewTransaction {
				conflictSetStat.HasTransaction = true
			}
			if resp.NewSignature {
				log.Debugf("%s sig count: %d", gn.ID(), conflictSetStat.SignatureCount)
				conflictSetStat.SignatureCount++
			}
			if resp.IsDone {
				conflictSetStat.IsDone = true
			}

			roundInfo, err := gn.Group.MostRecentRoundInfo(gn.Group.RoundAt(time.Now()))
			if err != nil {
				log.Errorf("%s could not fetch roundinfo %v", gn.ID(), err)
			}

			if roundInfo != nil && int64(conflictSetStat.SignatureCount) >= roundInfo.SuperMajorityCount() {
				log.Debugf("%s reached super majority", gn.ID())
				signatureMessage, err := gn.checkSignatures(resp.ConflictSetID)

				if err != nil {
					log.Errorf("%s error for checkSignatureCounts: %v", gn.ID(), err)
				}

				log.Debugf("queueing up done message")
				if signatureMessage != nil {
					conflictSetStat.PendingMessages = append([]ProvideMessage{*signatureMessage}, conflictSetStat.PendingMessages...)
				}
			}

			if len(conflictSetStat.PendingMessages) > 0 {
				log.Debugf("%s pending messages, queuing one", gn.ID())
				msg, remainingMessages := conflictSetStat.PendingMessages[0:1], conflictSetStat.PendingMessages[1:]
				conflictSetStat.PendingMessages = remainingMessages
				go func(msg ProvideMessage) {
					log.Debugf("%s queuing popped message: %s", gn.ID(), msg.Key)
					gn.newObjCh <- msg
				}(msg[0])
			}

		case <-gn.stopChan:
			return
		}
	}
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
	gn.newObjCh <- ProvideMessage{Key: t.StoredID(), Value: encodedTrans}
	return t.ID(), nil
}

func (gn *GossipNode) ID() string {
	if gn.id != "" {
		return gn.id
	}
	gn.id = gn.Host.P2PIdentity()
	return gn.id
}

func (gn *GossipNode) isMessageDone(msg ProvideMessage) (bool, error) {
	return isDone(gn.Storage, msg.Key[len(msg.Key)-32:])
}

func (gn *GossipNode) checkSignatures(conflictSetID []byte) (*ProvideMessage, error) {
	conflictSetKeys, err := gn.Storage.GetKeysByPrefix(conflictSetID[0:4])
	if err != nil {
		return nil, fmt.Errorf("error fetching keys %v", err)
	}
	var matchedSigKeys [][]byte

	for _, k := range conflictSetKeys {
		if k[8] == byte(MessageTypeSignature) && bytes.Equal(conflictSetIDFromMessageKey(k), conflictSetID) {
			matchedSigKeys = append(matchedSigKeys, k)
		}
	}

	roundInfo, err := gn.Group.MostRecentRoundInfo(gn.Group.RoundAt(time.Now()))
	if err != nil {
		return nil, fmt.Errorf("error fetching roundinfo %v", err)
	}

	if int64(len(matchedSigKeys)) < roundInfo.SuperMajorityCount() {
		return nil, nil
	}

	signatures, err := gn.Storage.GetAll(matchedSigKeys)
	if err != nil {
		return nil, fmt.Errorf("error fetching signers %v", err)
	}

	signersByTransactionID := make(map[string]map[string][]byte)

	for _, sigBytes := range signatures {
		var s Signature
		_, err = s.UnmarshalMsg(sigBytes.Value)
		if err != nil {
			return nil, fmt.Errorf("error unmarshaling sig: %v", err)
		}
		for k, v := range s.Signers {
			if v == true {
				if _, ok := signersByTransactionID[string(s.TransactionID)]; !ok {
					signersByTransactionID[string(s.TransactionID)] = make(map[string][]byte)
				}
				signersByTransactionID[string(s.TransactionID)][k] = s.Signature
			}
		}
	}

	var majorityTransactionID []byte
	var majoritySigners map[string][]byte

	for transactionID, signers := range signersByTransactionID {
		if int64(len(signers)) >= roundInfo.SuperMajorityCount() {
			majorityTransactionID = []byte(transactionID)
			majoritySigners = signers
			break
		}
	}

	if len(majoritySigners) == 0 {
		return nil, nil
	}

	allSigners := make(map[string]bool)
	allSignatures := make([][]byte, 0)

	for _, signer := range roundInfo.Signers {
		key := consensus.BlsVerKeyToAddress(signer.VerKey.PublicKey).String()

		if sig, ok := majoritySigners[key]; ok {
			allSignatures = append(allSignatures, sig)
			allSigners[key] = true
		} else {
			allSigners[key] = false
		}
	}

	combinedSignatures, err := bls.SumSignatures(allSignatures)
	if err != nil {
		return nil, fmt.Errorf("error combining sigs %v", err)
	}

	commitSignature := Signature{
		TransactionID: majorityTransactionID,
		Signers:       allSigners,
		Signature:     combinedSignatures,
	}

	encodedSig, err := commitSignature.MarshalMsg(nil)
	if err != nil {
		return nil, fmt.Errorf("error marshaling sig: %v", err)
	}
	doneID := doneIDFromConflictSetID(conflictSetID)

	log.Debugf("%s adding done sig %v", gn.ID(), doneID)
	doneMessage := ProvideMessage{
		Key:   doneID,
		Value: encodedSig,
	}

	return &doneMessage, nil
}

func (gn *GossipNode) Add(key, value []byte) (bool, error) {
	log.Debugf("%s adding %v", gn.ID(), key)
	didSet, err := gn.Storage.SetIfNotExists(key, value)
	if err != nil {
		log.Errorf("%s error setting: %v", gn.ID(), err)
		return false, fmt.Errorf("error setting storage: %v", err)
	}
	if didSet {
		ibfObjectID := byteToIBFsObjectId(key[0:8])
		gn.ibfSyncer.Lock()
		gn.Strata.Add(ibfObjectID)
		for _, filter := range gn.IBFs {
			filter.Add(ibfObjectID)
			// debugger := filter.GetDebug()
			// for k, cnt := range debugger {
			// 	if cnt > 1 {
			// 		panic(fmt.Sprintf("%s added a duplicate key: %v with uint64 %d", gn.ID(), key, k))
			// 	}
			// }
		}
		gn.ibfSyncer.Unlock()
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

func (gn *GossipNode) HandleNewTransactionProtocol(stream net.Stream) {
	err := DoNewTransactionProtocol(gn, stream)
	if err != nil {
		log.Errorf("error handling new transaction protocol: %v", err)
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
