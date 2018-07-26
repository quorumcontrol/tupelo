package gossip

import (
	"crypto/ecdsa"

	"fmt"

	"time"

	"crypto/rand"
	"math/big"

	"context"

	"bytes"

	"sync"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/google/uuid"
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

var CurrentStateBucket = []byte("accepted")
var TransactionBucket = []byte("transactions")
var TransactionToObjectBucket = []byte("transToObject")
var ToGossipBucket = []byte("toGossip")
var FinishedBucket = []byte("finishedTransaction")
var LockBucket = []byte("locks")

var TrueByte = []byte{byte(int8(1))}
var RejectedByte = []byte("R")

const (
	ctxRequestKey = iota
	ctxStartKey   = iota
)

type Handler interface {
	DoRequest(dst *ecdsa.PublicKey, req *network.Request) (chan *network.Response, error)
	AssignHandler(requestType string, handlerFunc network.HandlerFunc) error
	Start()
	Stop()
}

type TransactionId []byte
type ObjectId []byte

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

type StateHandler func(ctx context.Context, group *consensus.Group, objectId, transaction, currentState []byte) (nextState []byte, accepted bool, err error)
type AcceptedHandler func(ctx context.Context, group *consensus.Group, objectId, transaction, newState []byte) (err error)
type RejectedHandler func(ctx context.Context, group *consensus.Group, objectId, transaction, currentState []byte) (err error)

type Gossiper struct {
	MessageHandler     Handler
	Id                 string
	SignKey            *bls.SignKey
	Group              *consensus.Group
	Storage            storage.Storage
	StateHandler       StateHandler
	AcceptedHandler    AcceptedHandler
	RejectedHandler    RejectedHandler
	NumberOfGossips    int
	TimeBetweenGossips int
	checkAcceptedChan  chan TransactionId
	startGossipChan    chan TransactionId
	stopGossipChan     chan TransactionId
	stopChan           chan bool
	gossipChan         chan bool
	started            bool
	lockLock           *sync.RWMutex
}

type GossiperOpts struct {
	Id                 string
	MessageHandler     Handler
	SignKey            *bls.SignKey
	Group              *consensus.Group
	Storage            storage.Storage
	StateHandler       StateHandler
	AcceptedHandler    AcceptedHandler
	RejectedHandler    RejectedHandler
	NumberOfGossips    int
	TimeBetweenGossips int
}

func NewGossiper(opts *GossiperOpts) *Gossiper {
	g := &Gossiper{
		Id:                 opts.Id,
		MessageHandler:     opts.MessageHandler,
		SignKey:            opts.SignKey,
		Group:              opts.Group,
		Storage:            opts.Storage,
		StateHandler:       opts.StateHandler,
		AcceptedHandler:    opts.AcceptedHandler,
		RejectedHandler:    opts.RejectedHandler,
		NumberOfGossips:    opts.NumberOfGossips,
		TimeBetweenGossips: opts.TimeBetweenGossips,
		checkAcceptedChan:  make(chan TransactionId, 200),
		startGossipChan:    make(chan TransactionId, 100),
		stopGossipChan:     make(chan TransactionId, 1),
		stopChan:           make(chan bool, 1),
		gossipChan:         make(chan bool, 1),
		lockLock:           &sync.RWMutex{},
	}
	g.Initialize()
	return g
}

func (g *Gossiper) Initialize() {
	g.Storage.CreateBucketIfNotExists(CurrentStateBucket)
	g.Storage.CreateBucketIfNotExists(FinishedBucket)
	g.Storage.CreateBucketIfNotExists(TransactionBucket)
	g.Storage.CreateBucketIfNotExists(TransactionToObjectBucket)
	g.Storage.CreateBucketIfNotExists(ToGossipBucket)
	g.Storage.CreateBucketIfNotExists(LockBucket)
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
	if g.started {
		return
	}
	g.started = true
	g.MessageHandler.Start()
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
				err := g.handleCheckAccepted(context.Background(), transId)
				if err != nil {
					log.Error("error checking accepted", "g", g.Id, "err", err)
				}
			case <-g.gossipChan:
				go func() {
					err := g.doAllGossips()
					if err != nil {
						log.Error("error doing gossips", "g", g.Id, "err", err)
					}
				}()
			}
		}
	}()
	g.gossipChan <- true
}

func (g *Gossiper) Stop() {
	if !g.started {
		return
	}
	g.stopChan <- true
	g.MessageHandler.Stop()
	g.started = false
}

func (g *Gossiper) doAllGossips() error {
	doneChans := make([]chan error, 0)
	err := g.Storage.ForEach(ToGossipBucket, func(id, _ []byte) error {
		doneChan := make(chan error, 1)
		doneChans = append(doneChans, doneChan)
		go func(id TransactionId, ch chan error) {
			doneChan <- g.DoOneGossipRound(id)
		}(id, doneChan)
		return nil
	})

	if err != nil {
		return fmt.Errorf("error forEach on ToGossipBucket: %v", err)
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
	<-time.After(time.Duration(randInt(g.TimeBetweenGossips)) * time.Millisecond)
	g.gossipChan <- true
}

func (g *Gossiper) DoOneGossipRound(id TransactionId) error {
	numberToGossip := min(len(g.Group.SortedMembers)-1, g.NumberOfGossips)
	if numberToGossip == 0 {
		defer func() { g.stopGossipChan <- id }()
		return nil
	}
	doneChans := make([]chan error, numberToGossip)

	log.Debug("gossiping", "g", g.Id, "number", numberToGossip)

	selected := make(map[string]bool)

	for i := 0; i < numberToGossip; i++ {
		doneChans[i] = make(chan error, 1)
		mem := g.Group.RandomMember()
		// make sure to not gossip with self or already chosen to gossip
		_, ok := selected[mem.Id]
		for ok || mem.Id == g.Id {
			mem = g.Group.RandomMember()
			_, ok = selected[mem.Id]
		}
		selected[mem.Id] = true

		go func(ch chan error, mem *consensus.RemoteNode) {
			sigs, _ := g.savedSignaturesFor(context.Background(), id)
			log.Debug("do one gossip", "g", g.Id, "dst", mem.Id, "sigCount", len(sigs))
			ch <- g.DoOneGossip(mem.DstKey, id)
		}(doneChans[i], mem)
	}
	for i := 0; i < numberToGossip; i++ {
		err := <-doneChans[i]
		close(doneChans[i])
		if err != nil {
			return fmt.Errorf("error doing gossip round: %v", err)
		}
	}

	defer func() { g.checkAcceptedChan <- id }()
	return nil
}

func (g *Gossiper) DoOneGossip(dst consensus.PublicKey, id TransactionId) error {
	ctxId := uuid.New().String()
	ctx := context.WithValue(context.Background(), ctxRequestKey, "DoOneGossip-"+ctxId)
	ctx = context.WithValue(ctx, ctxStartKey, time.Now())

	sigs, err := g.savedSignaturesFor(ctx, id)
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

	log.Trace("sending gossip", "g", g.Id, "dst", crypto.PubkeyToAddress(*crypto.ToECDSAPub(dst.PublicKey)).String())
	start := time.Now()
	resp, err := g.MessageHandler.DoRequest(crypto.ToECDSAPub(dst.PublicKey), req)
	if err != nil {
		return fmt.Errorf("error doing request: %v", err)
	}

	gossipResp := &GossipMessage{}
	wireResp := <-resp
	took := time.Now().Sub(start)
	if took > (300 * time.Millisecond) {
		log.Error("gossip resp", "g", g.Id, "dst", crypto.PubkeyToAddress(*crypto.ToECDSAPub(dst.PublicKey)).String(), "took", took)
	}

	err = cbornode.DecodeInto(wireResp.Payload, gossipResp)
	if err != nil {
		return fmt.Errorf("error decoding: %v", err)
	}

	log.Debug("gossip response received", "g", g.Id, "sigCount", len(gossipResp.Signatures))

	err = g.saveVerifiedSignatures(ctx, gossipResp)
	if err != nil {
		return fmt.Errorf("error saving: %v", err)
	}

	return nil
}

func (g *Gossiper) HandleGossipRequest(ctx context.Context, req network.Request, respChan network.ResponseChan) error {
	ctx = context.WithValue(ctx, ctxRequestKey, req.Id)
	ctx = context.WithValue(ctx, ctxStartKey, time.Now())

	gossipMessage := &GossipMessage{}
	err := cbornode.DecodeInto(req.Payload, gossipMessage)
	if err != nil {
		return fmt.Errorf("error decoding message: %v", err)
	}
	log.Debug("handling gossip", "g", g.Id, "sigCount", len(gossipMessage.Signatures), "uuid", req.Id, "elapsed", elapsedTime(ctx))

	transactionId := gossipMessage.Id()
	g.Storage.CreateBucketIfNotExists(transactionId)

	ownSig, err := g.getSignature(ctx, transactionId, g.Id)
	if err != nil {
		return fmt.Errorf("error getting own sig: %v", err)
	}

	// if we haven't already seen this, then sign the new state after the transition
	// something like a "REJECT" state would be ok to sign too
	if ownSig == nil {
		log.Trace("ownSig nil", "g", g.Id, "uuid", req.Id, "elapsed", elapsedTime(ctx))

		//TODO: we need to check if the message is reasonable so that we can't DOS a chaintree
		isLocked, err := g.lockObject(gossipMessage.ObjectId)
		if err != nil {
			return fmt.Errorf("error locking: %v", err)
		}

		var nextState []byte

		if isLocked {
			currentState, err := g.GetCurrentState(gossipMessage.ObjectId)
			if err != nil {
				return fmt.Errorf("error getting current state")
			}
			handlerState, isAccepted, err := g.StateHandler(ctx, g.Group, gossipMessage.ObjectId, gossipMessage.Transaction, currentState.State)
			if err != nil {
				return fmt.Errorf("error calling state handler: %v", err)
			}

			if isAccepted {
				nextState = handlerState
			} else {
				nextState = RejectedByte
			}

		} else {
			log.Error("could not get lock on object", "g", g.Id)
			nextState = RejectedByte
		}

		err = g.saveOwnState(ctx, nextState, gossipMessage)
		if err != nil {
			log.Error("error saving: %v", err)
			return fmt.Errorf("error saving state: %v", err)
		}

		defer func() { g.startGossipChan <- transactionId }()
	}
	log.Trace("loading known sigs", "g", g.Id, "uuid", req.Id, "elapsed", elapsedTime(ctx))

	// now we have our own signature, get the sigs we already know about
	knownSigs, err := g.savedSignaturesFor(ctx, transactionId)
	if err != nil {
		return fmt.Errorf("error saving sigs: %v", err)
	}

	// and then save the verified gossiped sigs
	log.Trace("saving sigs", "g", g.Id, "uuid", req.Id, "elapsed", elapsedTime(ctx))
	err = g.saveVerifiedSignatures(ctx, gossipMessage)
	if err != nil {
		return fmt.Errorf("error saving verified signatures: %v", err)
	}

	respMessage := &GossipMessage{
		ObjectId:    gossipMessage.ObjectId,
		Transaction: gossipMessage.Transaction,
		Signatures:  knownSigs,
	}

	log.Debug("responding", "g", g.Id, "sigCount", len(knownSigs), "uuid", req.Id, "elapsed", elapsedTime(ctx))

	defer func() { g.checkAcceptedChan <- transactionId }()

	resp, err := network.BuildResponse(req.Id, 200, respMessage)
	if err != nil {
		return fmt.Errorf("error building response: %v", err)
	}

	respChan <- resp

	return nil
}

func (g *Gossiper) saveOwnState(ctx context.Context, nextState []byte, gossipMessage *GossipMessage) error {

	sig, err := consensus.BlsSign(nextState, g.SignKey)
	if err != nil {
		return fmt.Errorf("error signing next state: %v", err)
	}
	ownSig := &GossipSignature{
		State:     nextState,
		Signature: *sig,
	}

	err = g.saveTransactionFromMessage(ctx, gossipMessage)
	if err != nil {
		return fmt.Errorf("error saving transaction: %v", err)
	}

	err = g.saveSig(ctx, gossipMessage.Id(), g.Id, ownSig)
	if err != nil {
		return fmt.Errorf("error saving own sig: %v", err)
	}
	return nil
}

func (g *Gossiper) IsTransactionConsensed(id TransactionId) (bool, error) {
	timeBytes, err := g.Storage.Get(FinishedBucket, id)
	if err != nil {
		return false, fmt.Errorf("error getting accepted: %v", err)
	}
	if len(timeBytes) == 0 {
		return false, nil
	} else {
		return true, nil
	}
}

func (g *Gossiper) isObjectLocked(objectId ObjectId) (bool, error) {
	bytes, err := g.Storage.Get(LockBucket, objectId)
	if err != nil {
		return false, fmt.Errorf("error getting object: %v", err)
	}
	if len(bytes) == 0 {
		return false, nil
	}
	return true, nil
}

func (g *Gossiper) lockObject(objectId ObjectId) (bool, error) {

	g.lockLock.RLock()
	isLocked, err := g.isObjectLocked(objectId)
	if err != nil {
		g.lockLock.RUnlock()
		return false, fmt.Errorf("error getting locked: %v", err)
	}

	if isLocked {
		defer g.lockLock.RUnlock()
		return false, nil
	} else {
		g.lockLock.RUnlock()
		g.lockLock.Lock()
		defer g.lockLock.Unlock()

		err = g.Storage.Set(LockBucket, objectId, TrueByte)
		if err != nil {
			return false, fmt.Errorf("error setting lock: %v", err)
		}
		return true, nil
	}
}

func (g *Gossiper) unlockObject(objectId ObjectId) error {
	g.lockLock.Lock()
	defer g.lockLock.Unlock()

	return g.Storage.Delete(LockBucket, objectId)
}

func (g *Gossiper) saveVerifiedSignatures(ctx context.Context, gossipMessage *GossipMessage) error {
	log.Trace("saveVerifiedSignatures - begin", "g", g.Id, "uuid", ctx.Value(ctxRequestKey), "elapsed", elapsedTime(ctx))
	verifiedSigs, err := g.verifiedNewSigsFromMessage(ctx, gossipMessage)
	if err != nil {
		return fmt.Errorf("error verifying sigs: %v", err)
	}
	log.Trace("saveVerifiedSignatures - verified", "g", g.Id, "uuid", ctx.Value(ctxRequestKey), "elapsed", elapsedTime(ctx))

	for signer, sig := range verifiedSigs {
		err = g.saveSig(ctx, gossipMessage.Id(), signer, &sig)
		if err != nil {
			return fmt.Errorf("error saving sig: %v", err)
		}
	}
	return nil
}

func (g *Gossiper) savedSignaturesFor(ctx context.Context, transactionId TransactionId) (GossipSignatureMap, error) {
	sigMap := make(GossipSignatureMap)
	g.Storage.CreateBucketIfNotExists(transactionId) // TODO: shouldn't need this

	log.Trace("savedSignaturesFor", "g", g.Id, "id", string(transactionId))
	err := g.Storage.ForEach(transactionId, func(k, v []byte) error {
		sig, err := g.sigFromBytes(ctx, v)
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

func (g *Gossiper) verifiedNewSigsFromMessage(ctx context.Context, gossipMessage *GossipMessage) (GossipSignatureMap, error) {
	log.Trace("verifiedNewSigsFromMessage - Begin", "g", g.Id, "uuid", ctx.Value(ctxRequestKey), "elapsed", elapsedTime(ctx))
	sigMap := make(GossipSignatureMap)
	keys := g.Group.AsVerKeyMap()

	existingArray, err := g.Storage.GetKeys(gossipMessage.Id())
	if err != nil {
		return nil, fmt.Errorf("error getting keys: %v", err)
	}

	existing := make(map[string]bool)
	for _, k := range existingArray {
		existing[string(k)] = true
	}

	for signer, sig := range gossipMessage.Signatures {
		log.Trace("verifiedNewSigsFromMessage - oneSig begin", "g", g.Id, "uuid", ctx.Value(ctxRequestKey), "elapsed", elapsedTime(ctx))
		_, ok := existing[signer]
		if ok {
			continue
		}

		verKey, ok := keys[signer]
		if !ok {
			continue
		}

		isVerified, err := consensus.Verify(consensus.MustObjToHash(sig.State), sig.Signature, verKey)
		if err != nil {
			return nil, fmt.Errorf("error verifying: %v", err)
		}
		log.Trace("verifiedNewSigsFromMessage - oneSig complete", "g", g.Id, "uuid", ctx.Value(ctxRequestKey), "elapsed", elapsedTime(ctx))

		if isVerified {
			sigMap[signer] = sig
		} else {
			log.Error("bad sig", "g", g.Id, "signer", signer)
		}
	}

	return sigMap, nil
}

func (g *Gossiper) GetCurrentState(objectId []byte) (sig GossipSignature, err error) {
	stateBytes, err := g.Storage.Get(CurrentStateBucket, objectId)
	if err != nil {
		return sig, fmt.Errorf("error getting state bytes: %v", err)
	}

	if len(stateBytes) > 0 {
		err = cbornode.DecodeInto(stateBytes, &sig)
		if err != nil {
			return sig, fmt.Errorf("error decoding: %v", err)
		}
		return sig, nil
	} else {
		return sig, nil
	}
}

func (g *Gossiper) setCurrentState(objectId []byte, sigs GossipSignatureMap) error {
	consensusSigs := make(consensus.SignatureMap)
	var state []byte
	sw := &dag.SafeWrap{}

	for signer, gossipSig := range sigs {
		consensusSigs[signer] = gossipSig.Signature
		state = gossipSig.State
	}

	combinedSig, err := g.Group.CombineSignatures(consensusSigs)
	if err != nil {
		return fmt.Errorf("error combining sigs: %v", err)
	}

	stateSig := &GossipSignature{
		State:     state,
		Signature: *combinedSig,
	}

	node := sw.WrapObject(stateSig)
	if sw.Err != nil {
		return fmt.Errorf("error wrapping obj: %v", err)
	}

	err = g.Storage.Set(CurrentStateBucket, objectId, node.RawData())
	if err != nil {
		return fmt.Errorf("error setting storage: %v", err)
	}

	return nil
}

func (g *Gossiper) getSignature(ctx context.Context, transactionId TransactionId, signer string) (*GossipSignature, error) {
	sigBytes, err := g.Storage.Get(transactionId, []byte(signer))
	if err != nil {
		return nil, fmt.Errorf("error getting self-sig: %v", err)
	}

	return g.sigFromBytes(ctx, sigBytes)
}

func (g *Gossiper) sigFromBytes(_ context.Context, sigBytes []byte) (*GossipSignature, error) {
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

func (g *Gossiper) saveSig(ctx context.Context, transactionId TransactionId, signer string, sig *GossipSignature) error {
	log.Trace("saveSig", "req", ctx.Value(ctxRequestKey), "elapsed", elapsedTime(ctx))
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

func (g *Gossiper) saveTransactionFromMessage(_ context.Context, msg *GossipMessage) error {
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

func (g *Gossiper) handleCheckAccepted(ctx context.Context, id TransactionId) error {
	acceptedBytes, err := g.Storage.Get(FinishedBucket, id)
	if err != nil {
		return fmt.Errorf("error checking bucket: %v", err)
	}
	if len(acceptedBytes) > 0 {
		return nil
	}

	sigs, err := g.savedSignaturesFor(ctx, id)
	if err != nil {
		return fmt.Errorf("error getting sigs: %v", err)
	}

	required := g.Group.SuperMajorityCount()
	if int64(len(sigs)) < required {
		return nil
	}

	obj, err := g.getObjectForTransaction(id)
	if err != nil {
		return fmt.Errorf("error getting object %v", err)
	}

	log.Debug("checking for accepted", "g", g.Id, "sigCount", len(sigs), "required", required)

	states := make(map[string]GossipSignatureMap)

	for signer, sig := range sigs {
		sigMap, ok := states[string(sig.State)]
		if !ok {
			sigMap = make(GossipSignatureMap)
		}
		sigMap[signer] = sig
		states[string(sig.State)] = sigMap
	}

	for state, sigs := range states {
		if int64(len(sigs)) >= required {
			log.Debug("super majority", "g", g.Id, "state", string(state))
			// we have a super majority!

			if bytes.Equal(RejectedByte, []byte(state)) {
				if g.RejectedHandler != nil {
					trans, err := g.getTransaction(id)
					if err != nil {
						return fmt.Errorf("error getting transaction from storage: %v", err)
					}

					curr, err := g.GetCurrentState(obj)
					if err != nil {
						return fmt.Errorf("error getting current state: %v", err)
					}
					err = g.RejectedHandler(ctx, g.Group, obj, trans, curr.State)
				}
			} else {
				err = g.setCurrentState(obj, sigs)
				if err != nil {
					return fmt.Errorf("error setting current state bucket: %v", err)
				}

				if g.AcceptedHandler != nil {
					trans, err := g.getTransaction(id)
					if err != nil {
						return fmt.Errorf("error getting transaction from storage: %v", err)
					}

					err = g.AcceptedHandler(ctx, g.Group, obj, trans, []byte(state))
					if err != nil {
						log.Error("error calling accepted", "err", err)
						return fmt.Errorf("error calling accepted: %v", err)
					}
				}
			}

			err = g.unlockObject(obj)
			if err != nil {
				return fmt.Errorf("error unlocking object: %v", err)
			}

			err = g.Storage.Set(FinishedBucket, id, nowBytes())
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

func nowBytes() []byte {
	t, _ := time.Now().UTC().MarshalBinary()
	return t
}

func min(a, b int) int {
	if a < b {
		return a
	} else {
		return b
	}
}

func randInt(max int) int {
	bigInt, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		log.Error("error reading rand", "err", err)
	}
	return int(bigInt.Int64())
}

func elapsedTime(ctx context.Context) time.Duration {
	start := ctx.Value(ctxStartKey)
	if start == nil {
		start = 0
	}
	return time.Now().Sub(start.(time.Time))
}
