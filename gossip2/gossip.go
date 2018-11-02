package gossip2

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/binary"
	"fmt"
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

const minSyncNodesPerTransaction = 3

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
		newObjCh:  make(chan ProvideMessage, 5),
		stopChan:  make(chan struct{}, 1),
		ibfSyncer: &sync.RWMutex{},
		syncPool:  make(chan SyncHandlerWorker, NumberOfSyncWorkers),
	}
	node.verKey = node.SignKey.MustVerKey()
	node.address = consensus.BlsVerKeyToAddress(node.verKey.Bytes()).String()
	for i := 0; i < NumberOfSyncWorkers; i++ {
		node.syncPool <- SyncHandlerWorker{}
	}

	go node.handleNewObjCh()

	for _, size := range standardIBFSizes {
		node.IBFs[size] = ibf.NewInvertibleBloomFilter(size, 4)
		// node.IBFs[size].TurnOnDebug()
	}
	host.SetStreamHandler(syncProtocol, node.HandleSync)
	host.SetStreamHandler(IsDoneProtocol, node.HandleDoneProtocol)
	host.SetStreamHandler(NewTransactionProtocol, node.HandleNewTransactionProtocol)
	return node
}

func (gn *GossipNode) Start() {
	for {
		select {
		case <-gn.stopChan:
			return
		default:
			err := gn.DoSync()
			if err != nil {
				log.Errorf("%s error doing sync %v", gn.ID(), err)
			}
		}
	}
}

func (gn *GossipNode) Stop() {
	gn.stopChan <- struct{}{}
}

func (gn *GossipNode) handleNewObjCh() {
	for msg := range gn.newObjCh {
		gn.processNewProvideMessage(msg)
	}
}

func (gn *GossipNode) InitiateTransaction(t Transaction) ([]byte, error) {
	encodedTrans, err := t.MarshalMsg(nil)
	if err != nil {
		return nil, fmt.Errorf("error encoding: %v", err)
	}
	log.Debugf("%s initiating transaction %v", gn.ID(), t.StoredID())
	return t.StoredID(), gn.handleNewTransaction(ProvideMessage{Key: t.StoredID(), Value: encodedTrans})
}

func (gn *GossipNode) ID() string {
	if gn.id != "" {
		return gn.id
	}
	gn.id = gn.Host.P2PIdentity()
	return gn.id
}

func (gn *GossipNode) processNewProvideMessage(msg ProvideMessage) {
	if msg.Last {
		log.Debugf("%s: handling a new EndOfStream message", gn.ID())
		return
	}

	exists, err := gn.Storage.Exists(msg.Key)
	if err != nil {
		log.Errorf("%s error seeing if element exists: %v, key: %v", gn.ID(), err, msg.Key)
		panic(fmt.Sprintf("error seeing if element exists: %v", err))
	}
	if !exists {
		log.Debugf("%s storage key NO EXIST %v", gn.ID(), msg.Key)

		if useDoneRemovalFilter {
			if isDone, _ := gn.isMessageDone(msg); isDone {
				return
			}
		}

		messageType := MessageType(msg.Key[8])
		switch messageType {
		case MessageTypeSignature:
			log.Debugf("%v: handling a new Signature message", gn.ID())
			gn.handleNewSignature(msg)
		case MessageTypeTransaction:
			log.Debugf("%v: handling a new Transaction message", gn.ID())
			gn.handleNewTransaction(msg)
		case MessageTypeDone:
			log.Debugf("%v: handling a new Done message", gn.ID())
			gn.handleDone(msg)
		default:
			log.Errorf("%v: unknown message %v", gn.ID(), msg.Key)
		}
	}
}

func (gn *GossipNode) isMessageDone(msg ProvideMessage) (bool, error) {
	return isDone(gn.Storage, msg.Key[:32])
}

func (gn *GossipNode) handleDone(msg ProvideMessage) error {
	log.Debugf("%s adding done message %v", gn.ID(), msg.Key)
	gn.Add(msg.Key, msg.Value)

	conflictSetKeys, err := gn.Storage.GetKeysByPrefix(msg.Key[0:4])

	for _, key := range conflictSetKeys {
		if key[8] != byte(MessageTypeDone) {
			gn.Remove(key)
			if err != nil {
				log.Errorf("%s error deleting conflict set item %v", gn.ID(), key)
			}
		}
	}

	return nil
}

func (gn *GossipNode) handleNewTransaction(msg ProvideMessage) error {
	//TODO: check if transaction hash matches key hash
	//TODO: check if the conflict set is done
	//TODO: sign this transaction if it's valid and new

	var t Transaction
	_, err := t.UnmarshalMsg(msg.Value)

	if err != nil {
		return fmt.Errorf("error getting transaction: %v", err)
	}
	log.Debugf("%s new transaction %s", gn.ID(), bytesToString(t.ID()))

	isValid, err := gn.IsTransactionValid(t)
	if err != nil {
		return fmt.Errorf("error validating transaction: %v", err)
	}

	if isValid {
		//TODO: sign the right thing
		sig, err := gn.SignKey.Sign(msg.Value)
		if err != nil {
			return fmt.Errorf("error signing key: %v", err)
		}
		signature := Signature{
			TransactionID: t.ID(),
			Signers:       map[string]bool{gn.address: true},
			Signature:     sig,
		}
		encodedSig, err := signature.MarshalMsg(nil)
		if err != nil {
			return fmt.Errorf("error marshaling sig: %v", err)
		}
		log.Debugf("%s adding transaction %v", gn.ID(), msg.Key)
		gn.Add(msg.Key, msg.Value)
		log.Debugf("%s adding signature %v", gn.ID(), signature.StoredID(t.ToConflictSet().ID()))
		sigID := signature.StoredID(t.ToConflictSet().ID())
		gn.Add(sigID, encodedSig)

		sigMessage := ProvideMessage{
			Key:   sigID,
			Value: encodedSig,
		}

		gn.checkSignatureCounts(sigMessage)
	} else {
		log.Debugf("%s error, invalid transaction", gn.ID())
	}

	return nil
}

func (gn *GossipNode) handleNewSignature(msg ProvideMessage) error {
	log.Debugf("%s adding signature: %v", gn.ID(), msg.Key)
	if err := gn.checkSignatureCounts(msg); err != nil {
		return err
	}
	if useDoneRemovalFilter {
		isDone, _ := gn.isMessageDone(msg)
		if !isDone {
			gn.Add(msg.Key, msg.Value)
		}
	} else {
		gn.Add(msg.Key, msg.Value)
	}

	return nil
}

func (gn *GossipNode) checkSignatureCounts(msg ProvideMessage) error {
	transId := transactionIDFromSignatureKey(msg.Key)

	transBytes, err := gn.Storage.Get(transId)
	if err != nil {
		return fmt.Errorf("error getting transaction")
	}
	if len(transBytes) == 0 {
		log.Infof("%s signature received, but don't have transaction %s yet, msgKey: %v", gn.ID(), bytesToString(transId), msg.Key)
		return nil
	}

	var t Transaction
	_, err = t.UnmarshalMsg(transBytes)
	if err != nil {
		return fmt.Errorf("error unmarshaling: %v", err)
	}

	conflictSetKeys, err := gn.Storage.GetKeysByPrefix(msg.Key[0:4])
	if err != nil {
		return fmt.Errorf("error fetching keys %v", err)
	}
	var matchedSigKeys [][]byte

	for _, k := range conflictSetKeys {
		if k[8] == byte(MessageTypeSignature) && bytes.Equal(transactionIDFromSignatureKey(k), transId) {
			matchedSigKeys = append(matchedSigKeys, k)
		}
	}

	roundInfo, err := gn.Group.MostRecentRoundInfo(gn.Group.RoundAt(time.Now()))
	if err != nil {
		return fmt.Errorf("error fetching roundinfo %v", err)
	}

	if int64(len(matchedSigKeys)) >= roundInfo.SuperMajorityCount() {
		log.Infof("%s found super majority of sigs %d", gn.ID(), len(matchedSigKeys))
		signatures, err := gn.Storage.GetAll(matchedSigKeys)
		if err != nil {
			return fmt.Errorf("error fetching signers %v", err)
		}

		signaturesByKey := make(map[string][]byte)

		for _, sigBytes := range signatures {
			var s Signature
			_, err = s.UnmarshalMsg(sigBytes.Value)
			if err != nil {
				log.Errorf("%s error unamrshaling %v", gn.ID(), err)
				return fmt.Errorf("error unmarshaling sig: %v", err)
			}
			for k, v := range s.Signers {
				if v == true {
					signaturesByKey[k] = s.Signature
				}
			}
		}

		allSigners := make(map[string]bool)
		allSignatures := make([][]byte, 0)

		for _, signer := range roundInfo.Signers {
			key := consensus.BlsVerKeyToAddress(signer.VerKey.PublicKey).String()

			if sig, ok := signaturesByKey[key]; ok {
				allSignatures = append(allSignatures, sig)
				allSigners[key] = true
			} else {
				allSigners[key] = false
			}
		}

		combinedSignatures, err := bls.SumSignatures(allSignatures)
		if err != nil {
			return fmt.Errorf("error combining sigs %v", err)
		}

		commitSignature := Signature{
			TransactionID: t.ID(),
			Signers:       allSigners,
			Signature:     combinedSignatures,
		}

		encodedSig, err := commitSignature.MarshalMsg(nil)
		if err != nil {
			return fmt.Errorf("error marshaling sig: %v", err)
		}
		doneID := t.ToConflictSet().DoneID()

		log.Debugf("%s adding done sig %v", gn.ID(), doneID)
		doneMessage := ProvideMessage{
			Key:   doneID,
			Value: encodedSig,
		}
		err = gn.handleDone(doneMessage)
		if err != nil {
			return fmt.Errorf("error moving to done: %v", err)
		}
	}

	//TODO: check if signature hash matches signature contents
	return nil
}

func (gn *GossipNode) IsTransactionValid(t Transaction) (bool, error) {
	// state, err := gn.Storage.Get(t.ObjectID)
	// if err != nil {
	// 	return false, fmt.Errorf("error getting state: %v", err)
	// }

	// here we would send transaction and state to a handler
	return true, nil
}

func (gn *GossipNode) Add(key, value []byte) {
	ibfObjectID := byteToIBFsObjectId(key[0:8])
	log.Debugf("%s adding %v with uint64 %d", gn.ID(), key, ibfObjectID)
	err := gn.Storage.Set(key, value)
	if err != nil {
		panic("storage failed")
	}
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
