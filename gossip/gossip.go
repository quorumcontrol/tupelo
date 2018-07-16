package gossip

import (
	"crypto/ecdsa"

	"fmt"

	"github.com/ethereum/go-ethereum/crypto"
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

type Handler interface {
	DoRequest(dst *ecdsa.PublicKey, req *network.Request) (chan *network.Response, error)
	AssignHandler(requestType string, handlerFunc network.HandlerFunc) error
}

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

func (gm *GossipMessage) Id() []byte {
	return crypto.Keccak256(gm.Transaction)
}

type StateHandler func(currentState []byte, transaction []byte) (nextState []byte, err error)

type Gossiper struct {
	MessageHandler Handler
	Id             string
	SignKey        *bls.SignKey
	Group          *consensus.Group
	Storage        storage.Storage
	StateHandler   StateHandler
}

func (g *Gossiper) Initialize() {
	g.Storage.CreateBucketIfNotExists(AcceptedBucket)
	g.Storage.CreateBucketIfNotExists(TransactionBucket)
	g.MessageHandler.AssignHandler(MessageType_Gossip, g.HandleGossipRequest)
	if g.Id == "" {
		g.Id = consensus.BlsVerKeyToAddress(g.SignKey.MustVerKey().Bytes()).String()
	}
}

func (g *Gossiper) HandleGossipRequest(req network.Request) (*network.Response, error) {
	gossipMessage := &GossipMessage{}
	err := cbornode.DecodeInto(req.Payload, gossipMessage)
	if err != nil {
		return nil, fmt.Errorf("error decoding message: %v", err)
	}

	transactionId := gossipMessage.Id()

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

		err = g.saveSig(transactionId, g.Id, ownSig)
		if err != nil {
			return nil, fmt.Errorf("error saving own sig: %v", err)
		}
	}

	// now we have our own signature, get the sigs we already know about
	knownSigs, err := g.savedSignaturesFor(transactionId)
	if err != nil {
		return nil, fmt.Errorf("error saving sigs: %v", err)
	}

	// and then save the verified gossiped sigs

	verifiedSigs, err := g.verifiedSigsFromMessage(gossipMessage)
	if err != nil {
		return nil, fmt.Errorf("error verifying sigs: %v", err)
	}

	for signer, sig := range verifiedSigs {
		err = g.saveSig(transactionId, signer, &sig)
		if err != nil {
			return nil, fmt.Errorf("error saving sig: %v", err)
		}
	}

	respMessage := &GossipMessage{
		ObjectId:    gossipMessage.ObjectId,
		Transaction: gossipMessage.Transaction,
		Signatures:  knownSigs,
	}

	return network.BuildResponse(req.Id, 200, respMessage)
}

func (g *Gossiper) savedSignaturesFor(transactionId []byte) (GossipSignatureMap, error) {
	sigMap := make(GossipSignatureMap)

	err := g.Storage.ForEach(transactionId, func(k, v []byte) error {
		sig, err := g.sigFromBytes(v)
		if err != nil {
			return fmt.Errorf("error decoding sig: %v", err)
		}
		sigMap[string(k)] = *sig
		return nil
	})
	if err != nil {
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

func (g *Gossiper) getSignature(transactionId []byte, signer string) (*GossipSignature, error) {
	g.Storage.CreateBucketIfNotExists(transactionId)

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

func (g *Gossiper) saveSig(transactionId []byte, signer string, sig *GossipSignature) error {
	sw := &dag.SafeWrap{}
	sigBytes := sw.WrapObject(sig)
	if sw.Err != nil {
		return fmt.Errorf("error wrapping sig: %v", sw.Err)
	}

	return g.Storage.Set(transactionId, []byte(signer), sigBytes.RawData())
}
