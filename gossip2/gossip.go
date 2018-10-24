package gossip2

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-ipld-cbor"
	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	"github.com/quorumcontrol/differencedigest/ibf"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/p2p"
	"github.com/tinylib/msgp/msgp"
)

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

type IBFMap map[int]*ibf.InvertibleBloomFilter

var standardIBFSizes = []int{2000, 20000}

type MessageType int

const (
	MessageTypeSignature MessageType = iota
	MessageTypeTransaction
	MessageTypeDone
)

type GossipNode struct {
	Key      *ecdsa.PrivateKey
	SignKey  *bls.SignKey
	Host     *p2p.Host
	Storage  *BadgerStorage
	Strata   *ibf.DifferenceStrata
	Group    *consensus.NotaryGroup
	IBFs     IBFMap
	newObjCh chan ProvideMessage
}

func NewGossipNode(key *ecdsa.PrivateKey, host *p2p.Host, storage *BadgerStorage) *GossipNode {
	node := &GossipNode{
		Key:     key,
		Host:    host,
		Storage: storage,
		Strata:  ibf.NewDifferenceStrata(),
		IBFs:    make(IBFMap),
		//TODO: examine the 5 here?
		newObjCh: make(chan ProvideMessage, 5),
	}
	go node.handleNewObjCh()

	for _, size := range standardIBFSizes {
		node.IBFs[size] = ibf.NewInvertibleBloomFilter(size, 4)
	}
	host.SetStreamHandler(syncProtocol, node.HandleSync)
	return node
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
	return t.StoredID(), gn.handleNewTransaction(ProvideMessage{Key: t.StoredID(), Value: encodedTrans})
}

func (gn *GossipNode) ID() string {
	return base64.StdEncoding.EncodeToString(crypto.FromECDSAPub(&gn.Key.PublicKey))[0:8]
}

func (gn *GossipNode) processNewProvideMessage(msg ProvideMessage) {
	if msg.Last {
		return
	}

	exists, err := gn.Storage.Exists(msg.Key)
	if err != nil {
		panic(fmt.Sprintf("error seeing if element exists: %v", err))
	}
	if !exists {
		// TODO: add real processing here
		messageType := MessageType(msg.Key[8])
		switch messageType {
		case MessageTypeSignature:
			gn.handleNewSignature(msg)
		case MessageTypeTransaction:
			gn.handleNewTransaction(msg)
		case MessageTypeDone:
			fmt.Printf("%v: handling a new Done message\n", gn.ID())
			gn.handleDone(msg)
		}
	}
}

func (gn *GossipNode) handleDone(msg ProvideMessage) error {
	conflictSetDoneExists, err := gn.Storage.Exists(msg.Key)
	if err != nil {
		return fmt.Errorf("error getting conflict: %v\n", err)
	}
	if conflictSetDoneExists {
		return nil
	}

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

	conflictSetDoneExists, err := gn.Storage.Exists(t.ToConflictSet().DoneID())
	if err != nil {
		return fmt.Errorf("error getting conflict: %v", err)
	}
	if conflictSetDoneExists {
		return nil
	}

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
		gn.Add(signature.StoredID(t.ToConflictSet().ID()), encodedSig)
		gn.Add(msg.Key, msg.Value)
	}

	return nil
}

func (gn *GossipNode) handleNewSignature(msg ProvideMessage) error {
	transId := transactionIDFromSignatureKey(msg.Key)

	transBytes, err := gn.Storage.Get(transId)
	if err != nil {
		return fmt.Errorf("error getting transaction")
	}
	if len(transBytes) == 0 {
		// if we don't have the transaction yet, then just put this in the back of the queue
		//TODO: we should only process this twice... if we get a sig before a trans FINE, but
		// we should process the trans in the same stream and if we didn't get it
		// from this queue then we can just drop this message
		gn.newObjCh <- msg
		return nil
	}
	gn.Add(msg.Key, msg.Value)

	conflictSetKeys, err := gn.Storage.GetKeysByPrefix(msg.Key[0:4])
	if err != nil {
		return fmt.Errorf("error fetching keys %v\n", err)
	}
	var matchedSigKeys [][]byte

	for _, k := range conflictSetKeys {
		if k[8] == byte(MessageTypeSignature) && bytes.Equal(transactionIDFromSignatureKey(k), transId) {
			matchedSigKeys = append(matchedSigKeys, k)
		}
	}

	roundInfo, err := gn.Group.MostRecentRoundInfo(gn.Group.RoundAt(time.Now()))
	if err != nil {
		return fmt.Errorf("error fetching roundinfo %v\n", err)
	}

	if int64(len(matchedSigKeys)) >= roundInfo.SuperMajorityCount() {
		signatures, err := gn.Storage.GetAll(matchedSigKeys)
		if err != nil {
			return fmt.Errorf("error fetching signers %v\n", err)
		}

		signaturesByKey := make(map[string][]byte)

		for _, sigBytes := range signatures {
			var s Signature
			s.UnmarshalMsg(sigBytes.Value)
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

		var t Transaction
		t.UnmarshalMsg(transBytes)

		combinedSignatures, err := bls.SumSignatures(allSignatures)
		if err != nil {
			return fmt.Errorf("error combining sigs %v\n", err)
		}

		doneID := t.ToConflictSet().DoneID()

		commitSignature := Signature{
			TransactionID: doneID,
			Signers:       allSigners,
			Signature:     combinedSignatures,
		}

		encodedSig, err := commitSignature.MarshalMsg(nil)
		if err != nil {
			return fmt.Errorf("error marshaling sig: %v\n", err)
		}

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
	err := gn.Storage.Set(key, value)
	if err != nil {
		panic("storage failed")
	}
	gn.Strata.Add(ibfObjectID)
	for _, filter := range gn.IBFs {
		filter.Add(ibfObjectID)
	}
}

func (gn *GossipNode) Remove(key []byte) {
	ibfObjectID := byteToIBFsObjectId(key[0:8])
	err := gn.Storage.Delete(key)
	if err != nil {
		panic("storage failed")
	}
	gn.Strata.Remove(ibfObjectID)
	for _, filter := range gn.IBFs {
		filter.Remove(ibfObjectID)
	}
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

func (gn *GossipNode) DoSync() error {
	peer, err := gn.RandomPeer()
	if err != nil {
		return fmt.Errorf("error getting peer: %v", err)
	}

	ctx := context.Background()
	stream, err := gn.Host.NewStream(ctx, peer.DstKey.ToEcdsaPub(), syncProtocol)
	if err != nil {
		return fmt.Errorf("error opening new stream: %v", err)
	}
	writer := msgp.NewWriter(stream)
	reader := msgp.NewReader(stream)

	err = gn.IBFs[2000].EncodeMsg(writer)
	if err != nil {
		return fmt.Errorf("error writing IBF: %v", err)
	}
	fmt.Println("flushing")
	writer.Flush()
	var wants WantMessage
	err = wants.DecodeMsg(reader)
	if err != nil {
		return fmt.Errorf("error reading wants")
	}
	fmt.Printf("got the wants!: %v", wants)
	for _, key := range wants.Keys {
		key := uint64ToBytes(key)
		objs, err := gn.Storage.GetPairsByPrefix(key)
		if err != nil {
			return fmt.Errorf("error getting objects: %v", err)
		}
		for _, kv := range objs {
			provide := &ProvideMessage{
				Key:   kv.Key,
				Value: kv.Value,
			}
			provide.EncodeMsg(writer)
		}
	}
	last := &ProvideMessage{Last: true}
	last.EncodeMsg(writer)
	writer.Flush()

	// now get the objects we need
	var isLastMessage bool
	for !isLastMessage {
		var provideMsg ProvideMessage
		provideMsg.DecodeMsg(reader)
		fmt.Printf("received a provide: %v", provideMsg)
		gn.newObjCh <- provideMsg
		isLastMessage = provideMsg.Last
	}
	fmt.Printf("sync complete")
	stream.Close()

	return nil
}

func (gn *GossipNode) HandleSync(stream net.Stream) {
	writer := msgp.NewWriter(stream)
	reader := msgp.NewReader(stream)

	var remoteIBF ibf.InvertibleBloomFilter
	err := remoteIBF.DecodeMsg(reader)
	if err != nil {
		panic(fmt.Sprintf("error: %v", err))
	}
	difference, err := gn.IBFs[2000].Subtract(&remoteIBF).Decode()
	if err != nil {
		panic(fmt.Sprintf("error getting diff: %f", err))
	}
	want := WantMessageFromDiff(difference.RightSet)
	err = want.EncodeMsg(writer)
	if err != nil {
		panic(fmt.Sprintf("error writing wants: %v", err))
	}
	writer.Flush()
	var isLastMessage bool
	for !isLastMessage {
		var provideMsg ProvideMessage
		provideMsg.DecodeMsg(reader)
		fmt.Printf("received a provide: %v", provideMsg)
		gn.newObjCh <- provideMsg
		isLastMessage = provideMsg.Last
	}

	toProvideAsWantMessage := WantMessageFromDiff(difference.LeftSet)
	for _, key := range toProvideAsWantMessage.Keys {
		key := uint64ToBytes(key)
		objs, err := gn.Storage.GetPairsByPrefix(key)
		if err != nil {
			panic(fmt.Sprintf("error getting objects: %v", err))
		}
		for _, kv := range objs {
			provide := &ProvideMessage{
				Key:   kv.Key,
				Value: kv.Value,
			}
			provide.EncodeMsg(writer)
		}
	}
	last := &ProvideMessage{Last: true}
	last.EncodeMsg(writer)
	writer.Flush()
	stream.Close()
}

func byteToIBFsObjectId(byteID []byte) ibf.ObjectId {
	if len(byteID) != 8 {
		panic("invalid byte id sent")
	}
	return ibf.ObjectId(binary.BigEndian.Uint64(byteID))
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
