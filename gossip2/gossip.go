package gossip2

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-ipld-cbor"
	logging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/quorumcontrol/differencedigest/ibf"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/p2p"
	"github.com/tinylib/msgp/msgp"
)

var log = logging.Logger("gossip")

var doneBytes = []byte("done")
var trueByte = []byte{byte(1)}

func init() {
	if len(doneBytes) != 4 {
		panic("doneBytes must be 4 bytes!")
	}
	cbornode.RegisterCborType(WantMessage{})
	cbornode.RegisterCborType(ProvideMessage{})
	cbornode.RegisterCborType(ibf.InvertibleBloomFilter{})
	cbornode.RegisterCborType(ibf.DifferenceStrata{})
}

const syncProtocol = "tupelo-gossip/v1"

const minSyncNodesPerTransaction = 3

type IBFMap map[int]*ibf.InvertibleBloomFilter

var standardIBFSizes = []int{2000, 20000}

type MessageType int

const (
	MessageTypeSignature MessageType = iota + 1
	MessageTypeTransaction
	MessageTypeDone
)

type GossipNode struct {
	Key           *ecdsa.PrivateKey
	SignKey       *bls.SignKey
	Host          *p2p.Host
	Storage       *BadgerStorage
	Strata        *ibf.DifferenceStrata
	Group         *consensus.NotaryGroup
	IBFs          IBFMap
	newObjCh      chan ProvideMessage
	stopChan      chan struct{}
	syncTargetsCh chan *consensus.RemoteNode
	ibfSyncer     *sync.RWMutex
}

func NewGossipNode(key *ecdsa.PrivateKey, host *p2p.Host, storage *BadgerStorage) *GossipNode {
	node := &GossipNode{
		Key:     key,
		Host:    host,
		Storage: storage,
		Strata:  ibf.NewDifferenceStrata(),
		IBFs:    make(IBFMap),
		//TODO: examine the 5 here?
		newObjCh:      make(chan ProvideMessage, 5),
		syncTargetsCh: make(chan *consensus.RemoteNode, 50),
		stopChan:      make(chan struct{}, 1),
		ibfSyncer:     &sync.RWMutex{},
	}
	go node.handleNewObjCh()

	for _, size := range standardIBFSizes {
		node.IBFs[size] = ibf.NewInvertibleBloomFilter(size, 4)
		// node.IBFs[size].TurnOnDebug()
	}
	host.SetStreamHandler(syncProtocol, node.HandleSync)
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
		zeroCount := 0
		for i := len(msg.Key) - 1; i >= 0; i-- {
			b := msg.Key[i]
			if !bytes.Equal([]byte{b}, []byte{byte(0)}) {
				break
			}
			zeroCount++
		}
		if zeroCount > 5 {
			log.Errorf("%s all zero key %v", gn.ID(), msg.Key)
			panic("we're done in handleNewObjCh")
		}

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
	return gn.Host.P2PIdentity()
}

func (gn *GossipNode) processNewProvideMessage(msg ProvideMessage) {
	if msg.Last {
		log.Debugf("%s: handling a new EndOfStream message", gn.ID())
		return
	}
	zeroCount := 0
	for i := len(msg.Key) - 1; i >= 0; i-- {
		b := msg.Key[i]
		if !bytes.Equal([]byte{b}, []byte{byte(0)}) {
			break
		}
		zeroCount++
	}
	if zeroCount > 5 {
		log.Errorf("%s all zero key %v", gn.ID(), msg.Key)
		panic("we're done in processNewProvideMessage")
	}

	exists, err := gn.Storage.Exists(msg.Key)
	if err != nil {
		log.Errorf("%s error seeing if element exists: %v, key: %v", gn.ID(), err, msg.Key)
		panic(fmt.Sprintf("error seeing if element exists: %v", err))
	}
	if !exists {
		log.Debugf("%s storage key NO EXIST %v", gn.ID(), msg.Key)
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

func (gn *GossipNode) handleDone(msg ProvideMessage) error {
	conflictSetDoneExists, err := gn.Storage.Exists(msg.Key)
	if err != nil {
		return fmt.Errorf("error getting conflict: %v", err)
	}
	if conflictSetDoneExists {
		return nil
	}

	log.Debugf("%s adding done message %v", gn.ID(), msg.Key)
	gn.Add(msg.Key, msg.Value)

	// TODO: cleanout old transactions

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

	// conflictSetDoneExists, err := gn.Storage.Exists(t.ToConflictSet().DoneID())
	// if err != nil {
	// 	return fmt.Errorf("error getting conflict: %v", err)
	// }
	// if conflictSetDoneExists {
	// 	gn.Add(msg.Key, msg.Value)
	// 	return nil
	// }

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
			Signers:       map[string]bool{consensus.BlsVerKeyToAddress(gn.SignKey.MustVerKey().Bytes()).String(): true},
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
		gn.queueSyncTargetsByRoutingKey(t.NewTip)
	} else {
		log.Debugf("%s error, invalid transaction", gn.ID())
	}

	return nil
}

func (gn *GossipNode) handleNewSignature(msg ProvideMessage) error {
	log.Debugf("%s adding signature: %v", gn.ID(), msg.Key)
	gn.checkSignatureCounts(msg)
	gn.Add(msg.Key, msg.Value)
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
	doneID := t.ToConflictSet().DoneID()
	doneExists, err := gn.Storage.Exists(doneID)
	if err != nil {
		return fmt.Errorf("error getting exists: %v", err)
	}
	if doneExists {
		log.Debugf("%s already has done, skipping", gn.ID())
		return nil
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
		log.Debugf("%s adding done sig %v", gn.ID(), doneID)
		gn.Add(doneID, encodedSig)
	}

	//TODO: see if we have 2/3+1 of signatures
	//TODO: check if signature hash matches signature contents
	return nil
}

func (t *Transaction) ID() []byte {
	encoded, err := t.MarshalMsg(nil)
	if err != nil {
		panic("error marshaling transaction")
	}
	return crypto.Keccak256(encoded)
}

func (t *Transaction) ToConflictSet() *ConflictSet {
	return &ConflictSet{ObjectID: t.ObjectID, Tip: t.PreviousTip}
}

func (c *ConflictSet) ID() []byte {
	return crypto.Keccak256(concatBytesSlice(c.ObjectID, c.Tip))
}

func (c *ConflictSet) DoneID() []byte {
	conflictSetID := c.ID()
	return concatBytesSlice(conflictSetID[0:4], doneBytes, []byte{byte(MessageTypeDone)}, conflictSetID)
}

// ID in storage is 32bitsConflictSetId|32bitsTransactionHash|fulltransactionHash|"-transaction" or transaction before hash
func (t *Transaction) StoredID() []byte {
	conflictSetID := t.ToConflictSet().ID()
	id := t.ID()
	return concatBytesSlice(conflictSetID[0:4], id[0:4], []byte{byte(MessageTypeTransaction)}, id)
}

// transactionID conflictSet[0-3],transactionID[4-7],transactionByte[8],FulltransactionID[9-41]

// ID in storage is 32bitsConflictSetId|32bitssignaturehash|transactionid|signaturehash|"-signature" OR signature before transactionid or before signaturehash
func (s *Signature) StoredID(conflictSetID []byte) []byte {
	encodedSig, err := s.MarshalMsg(nil)
	if err != nil {
		panic("Could not marshal signature")
	}
	id := crypto.Keccak256(encodedSig)
	return concatBytesSlice(conflictSetID[0:4], id[0:4], []byte{byte(MessageTypeSignature)}, s.TransactionID, id)
}

// [254 216 115 227 ///// 35 11 251 183 //// 0 //// 57 79 66 166 142 30 188 4 223 174 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0]

//signatureID conflictSet[0-3],signatureHash[4-7],sigantureByte[8],transactionId[9-41],fullSignatureId[42-74]

func transactionIDFromSignatureKey(key []byte) []byte {
	return concatBytesSlice(key[0:4], key[9:13], []byte{byte(MessageTypeTransaction)}, key[9:41])
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

func (gn *GossipNode) RandomPeer() (*consensus.RemoteNode, error) {
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
	select {
	case peer, ok := <-gn.syncTargetsCh:
		if ok && peer != nil {
			log.Debugf("%s: getSyncTarget using peer from syncTargetsCh", gn.ID())
			return peer, nil
		}
	default:
	}
	log.Debugf("%s: getSyncTarget using random peer", gn.ID())
	return gn.RandomPeer()
}

func (gn *GossipNode) queueSyncTargetsByRoutingKey(key []byte) error {
	roundInfo, err := gn.Group.MostRecentRoundInfo(gn.Group.RoundAt(time.Now()))
	if err != nil {
		return fmt.Errorf("error fetching roundinfo %v", err)
	}

	signerCount := float64(len(roundInfo.Signers))
	logOfSigners := math.Log(signerCount)
	numberOfTargets := math.Floor(math.Max(logOfSigners, float64(minSyncNodesPerTransaction)))
	indexSpacing := signerCount / numberOfTargets
	moduloOffset := math.Mod(float64(bytesToUint64(key)), indexSpacing)

	for i := 0; i < int(numberOfTargets); i++ {
		targetIndex := int64(math.Floor(moduloOffset + (indexSpacing * float64(i))))
		select {
		case gn.syncTargetsCh <- roundInfo.Signers[targetIndex]:
		default:
			log.Debugf("%s: error pushing signer onto targets queue", gn.ID())
		}
	}

	return nil
}

func (gn *GossipNode) DoSync() error {
	log.Debugf("%s: sync started", gn.ID())

	peer, err := gn.getSyncTarget()
	if err != nil {
		return fmt.Errorf("error getting peer: %v", err)
	}
	peerPublicKey := bytesToString(peer.DstKey.PublicKey)[0:8]

	log.Debugf("%s: targeting peer %v", gn.ID(), peerPublicKey)

	ctx := context.Background()
	stream, err := gn.Host.NewStream(ctx, peer.DstKey.ToEcdsaPub(), syncProtocol)
	if err != nil {
		if err == p2p.ErrDialBackoff {
			log.Debugf("%s: dial backoff for peer %s", gn.ID(), peerPublicKey)
			return nil
		}
		return fmt.Errorf("%s: error opening new stream - %v", gn.ID(), err)
	}

	peerID := stream.Conn().RemotePeer().String()
	log.Debugf("%s peerKey %s established as %s", gn.ID(), peerPublicKey, peerID)

	writer := msgp.NewWriter(stream)
	reader := msgp.NewReader(stream)
	defer func() {
		writer.Flush()
		stream.Close()
	}()
	gn.ibfSyncer.RLock()
	err = gn.IBFs[20000].EncodeMsg(writer)
	gn.ibfSyncer.RUnlock()
	if err != nil {
		return fmt.Errorf("error writing IBF: %v", err)
	}
	log.Debugf("%s flushing", gn.ID())
	writer.Flush()
	var wants WantMessage
	err = wants.DecodeMsg(reader)
	if err != nil {
		log.Errorf("%s error reading wants %v", gn.ID(), err)
		// log.Errorf("%s ibf TO %s : %v", gn.ID(), peerID, gn.IBFs[2000].GetDebug())
		// log.Errorf("%s ibf TO %s cells : %v", gn.ID(), peerID, ibf.HumanizeIBF(gn.IBFs[2000]))

		return nil
	}
	log.Debugf("%s: got a want request for %v keys", gn.ID(), len(wants.Keys))
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
			log.Debugf("%s providing to %s (uint64: %d): %s %v", gn.ID(), peerID, key, bytesToString(provide.Key), provide.Key)
			provide.EncodeMsg(writer)
		}
	}
	log.Debugf("%s: sending Last message", gn.ID())
	last := ProvideMessage{Last: true}
	last.EncodeMsg(writer)
	writer.Flush()

	// now get the objects we need
	var isLastMessage bool
	for !isLastMessage {
		var provideMsg ProvideMessage
		err = provideMsg.DecodeMsg(reader)
		if err != nil {
			log.Errorf("%s error decoding message: %v", gn.ID(), err)
			return nil
		}
		if len(provideMsg.Key) == 0 && !provideMsg.Last {
			log.Errorf("%s error, got nil key, value: %v", gn.ID(), provideMsg.Key)
			return nil
		}
		log.Debugf("%s new msg from %s %s %v", gn.ID(), peerID, bytesToString(provideMsg.Key), provideMsg.Key)
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
	log.Debugf("%s: sync complete", gn.ID())

	return nil
}

func (gn *GossipNode) HandleSync(stream net.Stream) {
	peerID := stream.Conn().RemotePeer().String()
	log.Debugf("%s received sync request from %s", gn.ID(), peerID)

	writer := msgp.NewWriter(stream)
	reader := msgp.NewReader(stream)
	defer func() {
		writer.Flush()
		stream.Close()
	}()

	var remoteIBF ibf.InvertibleBloomFilter
	err := remoteIBF.DecodeMsg(reader)
	if err != nil {
		log.Errorf("%s error decoding message", gn.ID(), err)
		return
	}
	// log.Debugf("%s from %s received IBF (cells)", gn.ID(), peerID, ibf.HumanizeIBF(&remoteIBF))
	gn.ibfSyncer.RLock()
	subtracted := gn.IBFs[20000].Subtract(&remoteIBF)
	gn.ibfSyncer.RUnlock()
	difference, err := subtracted.Decode()
	if err != nil {
		log.Errorf("%s error getting diff): %f\n", gn.ID(), err)
		// log.Errorf("%s (talking to %s) local ibf is : %v", gn.ID(), peerID, gn.IBFs[2000].GetDebug())
		// log.Errorf("%s (talking to %s) local ibf cells : %v", gn.ID(), peerID, ibf.HumanizeIBF(gn.IBFs[2000]))
		return
	}
	log.Debugf("%s decoded", gn.ID())
	want := WantMessageFromDiff(difference.RightSet)
	err = want.EncodeMsg(writer)
	if err != nil {
		log.Errorf("%s error writing wants: %v", gn.ID(), err)
		return
	}
	writer.Flush()
	var isLastMessage bool
	for !isLastMessage {
		var provideMsg ProvideMessage
		err = provideMsg.DecodeMsg(reader)
		if err != nil {
			log.Errorf("%s error decoding message: %v", gn.ID(), err)
			return
		}
		log.Debugf("%s HandleSync provide message from %s - last? %v", gn.ID(), peerID, provideMsg.Last)
		if len(provideMsg.Key) == 0 && !provideMsg.Last {
			log.Errorf("%s HandleSync error, got nil key from %s, value: %v", gn.ID(), peerID, provideMsg.Key)
			return
		}
		log.Debugf("%s HandleSync new msg from %s %s %v", gn.ID(), peerID, bytesToString(provideMsg.Key), provideMsg.Key)

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
			panic("we're done in HandleSync")
		}

		gn.newObjCh <- provideMsg
		isLastMessage = provideMsg.Last
	}

	log.Debugf("%s: HandleSync received all provides from %s, moving forward", gn.ID(), peerID)

	toProvideAsWantMessage := WantMessageFromDiff(difference.LeftSet)
	for _, key := range toProvideAsWantMessage.Keys {
		key := uint64ToBytes(key)
		objs, err := gn.Storage.GetPairsByPrefix(key)
		if err != nil {
			log.Errorf("error getting objects: %v", err)
			return
		}
		for _, kv := range objs {
			provide := ProvideMessage{
				Key:   kv.Key,
				Value: kv.Value,
			}
			log.Debugf("%s HandleSync providing to %s (uint64: %d): %s %v", gn.ID(), peerID, key, bytesToString(provide.Key), provide.Key)
			provide.EncodeMsg(writer)
			writer.Flush()
		}
	}
	last := ProvideMessage{Last: true}
	last.EncodeMsg(writer)
	writer.Flush()
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
