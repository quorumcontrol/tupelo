package gossip

import (
	"crypto/ecdsa"

	"fmt"

	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/storage"
)

func init() {
	cbornode.RegisterCborType(GossipMessage{})
	cbornode.RegisterCborType(GossipSignature{})
}

const MessageType_Gossip = "GOSSIP"

var AcceptedBucket = []byte("accepted")
var TransactionBucket = []byte("transactions")
var TransactionToObjectBucket = []byte("transToObject")
var ToGossipBucket = []byte("toGossip")

var TrueByte = []byte{byte(int8(1))}

type Handler interface {
	DoRequest(dst *ecdsa.PublicKey, req *network.Request) (chan *network.Response, error)
	AssignHandler(requestType string, handlerFunc network.HandlerFunc) error
}

type TransactionId []byte

type GossipSignature struct {
	State     []byte
	Signature consensus.Signature
}

type GossipSignatureMap map[string]GossipSignature

type GossipMessage struct {
	ObjectId    []byte
	Transaction []byte
	Signatures  GossipSignatureMap
}

func (gm *GossipMessage) Id() TransactionId {
	return crypto.Keccak256(gm.Transaction)
}

type StateHandler func(currentState []byte, transaction []byte) (nextState []byte, err error)

type Gossiper struct {
	MessageHandler     Handler
	Id                 string
	SignKey            *bls.SignKey
	Group              *consensus.Group
	Storage            storage.Storage
	StateHandler       StateHandler
	NumberOfGossips    int
	TimeBetweenGossips int
	checkAcceptedChan  chan TransactionId
	startGossipChan    chan TransactionId
	stopGossipChan     chan TransactionId
	stopChan           chan bool
	gossipChan         chan bool
}

type GossiperOpts struct {
	MessageHandler     Handler
	SignKey            *bls.SignKey
	Group              *consensus.Group
	Storage            storage.Storage
	StateHandler       StateHandler
	NumberOfGossips    int
	TimeBetweenGossips int
}

func NewGossiper(opts *GossiperOpts) *Gossiper {
	g := &Gossiper{
		MessageHandler:     opts.MessageHandler,
		SignKey:            opts.SignKey,
		Group:              opts.Group,
		Storage:            opts.Storage,
		StateHandler:       opts.StateHandler,
		NumberOfGossips:    opts.NumberOfGossips,
		TimeBetweenGossips: opts.TimeBetweenGossips,
		checkAcceptedChan:  make(chan TransactionId, 1),
		startGossipChan:    make(chan TransactionId, 1),
		stopGossipChan:     make(chan TransactionId, 1),
		stopChan:           make(chan bool, 1),
		gossipChan:         make(chan bool, 1),
	}
	g.Initialize()
	return g
}

func (g *Gossiper) Initialize() {
	g.Storage.CreateBucketIfNotExists(AcceptedBucket)
	g.Storage.CreateBucketIfNotExists(TransactionBucket)
	g.Storage.CreateBucketIfNotExists(TransactionToObjectBucket)
	g.Storage.CreateBucketIfNotExists(ToGossipBucket)
	g.MessageHandler.AssignHandler(MessageType_Gossip, g.HandleGossipRequest)
	if g.Id == "" {
		g.Id = consensus.BlsVerKeyToAddress(g.SignKey.MustVerKey().Bytes()).String()
	}
	if g.NumberOfGossips == 0 {
		g.NumberOfGossips = 3 // good for approx 1k - 9k nodes
	}
	if g.TimeBetweenGossips == 0 {
		g.TimeBetweenGossips = 200 // 200 miliseconds
	}
}

func (g *Gossiper) Start() {
	go func() {
		for {
			select {
			case <-g.stopChan:
				return
			case transId := <-g.startGossipChan:
				g.handleStartGossip(transId)
			case transId := <-g.stopGossipChan:
				g.handleStopGossip(transId)
			case transId := <-g.checkAcceptedChan:
				err := g.handleCheckAccepted(transId)
				if err != nil {
					log.Error("error checking accepted", "g", g.Id, "err", err)
				}
			case <-g.gossipChan:
				err := g.doAllGossips()
				if err != nil {
					log.Error("error doing gossips", "g", g.Id, "err", err)
				}
			}
		}
	}()
	g.gossipChan <- true
}

func (g *Gossiper) Stop() {
	g.stopChan <- true
}

func (g *Gossiper) doAllGossips() error {
	doneChans := make([]chan error, 0)
	err := g.Storage.ForEach(ToGossipBucket, func(id, _ []byte) error {
		doneChan := make(chan error, 1)
		doneChans = append(doneChans, doneChan)
		go func() {
			doneChan <- g.DoOneGossipRound(id)
		}()
		return nil
	})

	if err != nil {
		return fmt.Errorf("error forEach on ToGossipBucket", "g", g.Id)
	}

	for i := 0; i < len(doneChans); i++ {
		err := <-doneChans[i]
		if err != nil {
			g.queueGossip()
			return fmt.Errorf("error doing gossip: %v", err)
		}
	}
	g.queueGossip()
	return nil
}

func (g *Gossiper) queueGossip() {
	<-time.After(200 * time.Millisecond)
	g.gossipChan <- true
}

func (g *Gossiper) DoOneGossipRound(id TransactionId) error {
	numberToGossip := min(len(g.Group.SortedMembers), g.NumberOfGossips)
	doneChans := make([]chan error, numberToGossip)

	log.Debug("gossiping", "g", g.Id, "number", numberToGossip)

	selected := make(map[string]bool)

	for i := 0; i < numberToGossip; i++ {
		doneChans[i] = make(chan error, 1)
		go func(ch chan error) {

			// make sure to choose distinct nodes in the random process
			mem := g.Group.RandomMember()
			_, ok := selected[mem.Id]
			for ok {
				mem = g.Group.RandomMember()
				_, ok = selected[mem.Id]
			}
			selected[mem.Id] = true

			ch <- g.DoOneGossip(mem.DstKey, id)
		}(doneChans[i])
	}
	for i := 0; i < numberToGossip; i++ {
		err := <-doneChans[i]
		close(doneChans[i])
		if err != nil {
			return fmt.Errorf("error doing gossip round: %v", err)
		}
	}

	go func() { g.checkAcceptedChan <- id }()
	return nil
}

func (g *Gossiper) DoOneGossip(dst consensus.PublicKey, id TransactionId) error {
	sigs, err := g.savedSignaturesFor(id)
	if err != nil {
		return fmt.Errorf("error getting saved signatures: %v", err)
	}

	obj, err := g.getObjectForTransaction(id)
	if err != nil {
		return fmt.Errorf("error getting object for id: %v", err)
	}

	trans, err := g.getTransaction(id)
	if err != nil {
		return fmt.Errorf("error getting transaction: %v", err)
	}

	msg := &GossipMessage{
		ObjectId:    obj,
		Transaction: trans,
		Signatures:  sigs,
	}

	req, err := network.BuildRequest(MessageType_Gossip, msg)
	if err != nil {
		return fmt.Errorf("error building request: %v", err)
	}

	resp, err := g.MessageHandler.DoRequest(crypto.ToECDSAPub(dst.PublicKey), req)
	if err != nil {
		return fmt.Errorf("error doing request: %v", err)
	}

	gossipResp := &GossipMessage{}
	err = cbornode.DecodeInto((<-resp).Payload, gossipResp)
	if err != nil {
		return fmt.Errorf("error decoding resp payload: %v", err)
	}

	err = g.saveVerifiedSignatures(gossipResp)
	if err != nil {
		return fmt.Errorf("error saving sigs: %v", err)
	}

	return nil
}

func (g *Gossiper) HandleGossipRequest(req network.Request) (*network.Response, error) {
	gossipMessage := &GossipMessage{}
	err := cbornode.DecodeInto(req.Payload, gossipMessage)
	if err != nil {
		return nil, fmt.Errorf("error decoding message: %v", err)
	}

	transactionId := gossipMessage.Id()
	g.Storage.CreateBucketIfNotExists(transactionId)

	ownSig, err := g.getSignature(transactionId, g.Id)
	if err != nil {
		return nil, fmt.Errorf("error getting own sig: %v", err)
	}

	// if we haven't already seen this, then sign the new state after the transition
	// something like a "REJECT" state would be ok to sign too
	if ownSig == nil {
		currentState, err := g.getCurrentState(gossipMessage.ObjectId)
		if err != nil {
			return nil, fmt.Errorf("error getting current state")
		}
		nextState, err := g.StateHandler(currentState, gossipMessage.Transaction)
		if err != nil {
			return nil, fmt.Errorf("error calling state handler: %v", err)
		}

		sig, err := consensus.BlsSign(nextState, g.SignKey)
		if err != nil {
			return nil, fmt.Errorf("error signing next state: %v", err)
		}
		ownSig = &GossipSignature{
			State:     nextState,
			Signature: *sig,
		}

		err = g.saveTransactionFromMessage(gossipMessage)
		if err != nil {
			return nil, fmt.Errorf("error saving transaction: %v", err)
		}

		err = g.saveSig(transactionId, g.Id, ownSig)
		if err != nil {
			return nil, fmt.Errorf("error saving own sig: %v", err)
		}
		defer func() { go func() { g.startGossipChan <- transactionId }() }()
	}

	// now we have our own signature, get the sigs we already know about
	knownSigs, err := g.savedSignaturesFor(transactionId)
	if err != nil {
		return nil, fmt.Errorf("error saving sigs: %v", err)
	}

	// and then save the verified gossiped sigs

	err = g.saveVerifiedSignatures(gossipMessage)
	if err != nil {
		return nil, fmt.Errorf("error saving verified signatures: %v", err)
	}

	respMessage := &GossipMessage{
		ObjectId:    gossipMessage.ObjectId,
		Transaction: gossipMessage.Transaction,
		Signatures:  knownSigs,
	}

	go func() { g.checkAcceptedChan <- transactionId }()

	return network.BuildResponse(req.Id, 200, respMessage)
}

func (g *Gossiper) saveVerifiedSignatures(gossipMessage *GossipMessage) error {
	verifiedSigs, err := g.verifiedSigsFromMessage(gossipMessage)
	if err != nil {
		return fmt.Errorf("error verifying sigs: %v", err)
	}

	for signer, sig := range verifiedSigs {
		err = g.saveSig(gossipMessage.Id(), signer, &sig)
		if err != nil {
			return fmt.Errorf("error saving sig: %v", err)
		}
	}
	return nil
}

func (g *Gossiper) savedSignaturesFor(transactionId TransactionId) (GossipSignatureMap, error) {
	sigMap := make(GossipSignatureMap)
	g.Storage.CreateBucketIfNotExists(transactionId) // TODO: shouldn't need this

	log.Debug("savedSignaturesFor", "g", g.Id, "id", string(transactionId))
	err := g.Storage.ForEach(transactionId, func(k, v []byte) error {
		sig, err := g.sigFromBytes(v)
		if err != nil {
			return fmt.Errorf("error decoding sig: %v", err)
		}
		sigMap[string(k)] = *sig
		return nil
	})
	if err != nil {
		log.Error("error getting saved sigs", "g", g.Id, "err", err)
		return nil, fmt.Errorf("error getting saved sigs: %v", err)
	}

	return sigMap, nil
}

func (g *Gossiper) verifiedSigsFromMessage(gossipMessage *GossipMessage) (GossipSignatureMap, error) {
	sigMap := make(GossipSignatureMap)
	keys := g.Group.AsVerKeyMap()

	for signer, sig := range gossipMessage.Signatures {
		verKey, ok := keys[signer]
		if !ok {
			continue
		}

		isVerified, err := consensus.Verify(consensus.MustObjToHash(sig.State), sig.Signature, verKey)
		if err != nil {
			return nil, fmt.Errorf("error verifying: %v", err)
		}

		if isVerified {
			sigMap[signer] = sig
		}
	}

	return sigMap, nil
}

func (g *Gossiper) getCurrentState(objectId []byte) ([]byte, error) {
	return g.Storage.Get(AcceptedBucket, objectId)
}

func (g *Gossiper) getSignature(transactionId TransactionId, signer string) (*GossipSignature, error) {
	sigBytes, err := g.Storage.Get(transactionId, []byte(signer))
	if err != nil {
		return nil, fmt.Errorf("error getting self-sig: %v", err)
	}

	return g.sigFromBytes(sigBytes)
}

func (g *Gossiper) sigFromBytes(sigBytes []byte) (*GossipSignature, error) {
	if len(sigBytes) == 0 {
		return nil, nil
	}

	gossipSig := &GossipSignature{}
	err := cbornode.DecodeInto(sigBytes, gossipSig)
	if err != nil {
		return nil, fmt.Errorf("error reconstituting sig: %v", err)
	}

	return gossipSig, nil
}

func (g *Gossiper) saveSig(transactionId TransactionId, signer string, sig *GossipSignature) error {
	sw := &dag.SafeWrap{}
	sigBytes := sw.WrapObject(sig)
	if sw.Err != nil {
		return fmt.Errorf("error wrapping sig: %v", sw.Err)
	}

	return g.Storage.Set(transactionId, []byte(signer), sigBytes.RawData())
}

func (g *Gossiper) getObjectForTransaction(id TransactionId) ([]byte, error) {
	return g.Storage.Get(TransactionToObjectBucket, id)
}

func (g *Gossiper) getTransaction(id TransactionId) ([]byte, error) {
	return g.Storage.Get(TransactionBucket, id)
}

func (g *Gossiper) saveTransactionFromMessage(msg *GossipMessage) error {
	err := g.Storage.Set(TransactionBucket, msg.Id(), msg.Transaction)
	if err != nil {
		return fmt.Errorf("error saving transaction: %v", err)
	}
	return g.Storage.Set(TransactionToObjectBucket, msg.Id(), msg.ObjectId)
}

func (g *Gossiper) handleStartGossip(id TransactionId) {
	log.Debug("start gossip", "g", g.Id, "id", string(id))
	g.Storage.Set(ToGossipBucket, id, TrueByte)
}

func (g *Gossiper) handleStopGossip(id TransactionId) {
	log.Debug("stop gossip", "g", g.Id, "id", string(id))
	g.Storage.Delete(ToGossipBucket, id)
}

func (g *Gossiper) handleCheckAccepted(id TransactionId) error {
	sigs, err := g.savedSignaturesFor(id)
	if err != nil {
		return fmt.Errorf("error getting sigs: %v", err)
	}

	required := g.Group.SuperMajorityCount()
	if int64(len(sigs)) < required {
		return nil
	}

	states := make(map[string][]*GossipSignature)

	for _, sig := range sigs {
		states[string(sig.State)] = append(states[string(sig.State)], &sig)
	}

	for state, sigs := range states {
		if int64(len(sigs)) > required {
			log.Debug("super majority", "g", g.Id, "state", string(state))
			// we have a super majority!

			obj, err := g.getObjectForTransaction(id)
			if err != nil {
				return fmt.Errorf("error getting object %v", err)
			}

			err = g.Storage.Set(AcceptedBucket, obj, []byte(state))
			if err != nil {
				return fmt.Errorf("error setting accepted bucket: %v", err)
			}

			//TODO: we need to stop before we have all the signatures
			if len(sigs) == len(g.Group.SortedMembers) {
				g.stopGossipChan <- id
			}
		}
	}

	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}
