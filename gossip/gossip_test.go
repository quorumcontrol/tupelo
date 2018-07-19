package gossip

import (
	"context"
	"crypto/ecdsa"
	"testing"

	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/network"
	"github.com/quorumcontrol/qc3/storage"
	"github.com/stretchr/testify/assert"
)

type testSet struct {
	SignKeys          []*bls.SignKey
	VerKeys           []*bls.VerKey
	EcdsaKeys         []*ecdsa.PrivateKey
	PubKeys           []consensus.PublicKey
	SignKeysByAddress map[string]*bls.SignKey
}

// This is the simplest possible state handler that just always returns the transaction as the nextState
func simpleHandler(_ context.Context, _ []byte, transaction []byte) (nextState []byte, err error) {
	return transaction, nil
}

type InMemoryHandlerSystem struct {
	Handlers          map[string]*InMemoryHandler
	ArtificialLatency int
}

func NewInMemoryHandlerSystem(latency int) *InMemoryHandlerSystem {
	return &InMemoryHandlerSystem{
		Handlers:          make(map[string]*InMemoryHandler),
		ArtificialLatency: latency,
	}
}

func (imhs *InMemoryHandlerSystem) NewHandler(dst *ecdsa.PublicKey) *InMemoryHandler {
	imh := &InMemoryHandler{
		Id:     crypto.PubkeyToAddress(*dst).String(),
		System: imhs,
	}
	imhs.Handlers[imh.Id] = imh
	return imh
}

type InMemoryHandler struct {
	Id      string
	System  *InMemoryHandlerSystem
	Mapping map[string]network.HandlerFunc
}

func (imh *InMemoryHandler) DoRequest(dst *ecdsa.PublicKey, req *network.Request) (chan *network.Response, error) {
	respChan := make(chan *network.Response, 1)
	start := time.Now()

	log.Trace("DoRequest to", "dst", crypto.PubkeyToAddress(*dst).String(), "uuid", req.Id, "start", start)
	go func(dst *ecdsa.PublicKey, req *network.Request) {
		log.Trace("DoRequest go func started", "dst", crypto.PubkeyToAddress(*dst).String(), "uuid", req.Id, "elapsed", time.Now().Sub(start))
		handler, ok := imh.System.Handlers[crypto.PubkeyToAddress(*dst).String()]
		if !ok {
			log.Error("could not find handler")
			panic("could not find handler")
		}
		resp, err := handler.Mapping[req.Type](context.Background(), *req)
		if err != nil {
			log.Error("error handling request: %v", err)
		}
		log.Trace("DoRequest func executed", "dst", crypto.PubkeyToAddress(*dst).String(), "uuid", req.Id, "elapsed", time.Now().Sub(start))
		<-time.After(time.Duration(imh.System.ArtificialLatency) * time.Millisecond)
		log.Trace("DoRequest responding", "dst", crypto.PubkeyToAddress(*dst).String(), "uuid", req.Id, "elapsed", time.Now().Sub(start))
		respChan <- resp
	}(dst, req)

	return respChan, nil
}

func (imh *InMemoryHandler) AssignHandler(requestType string, handlerFunc network.HandlerFunc) error {
	if imh.Mapping == nil {
		imh.Mapping = make(map[string]network.HandlerFunc)
	}

	imh.Mapping[requestType] = handlerFunc
	return nil
}

func newTestSet(t *testing.T, size int) *testSet {
	signKeys := blsKeys(size)
	verKeys := make([]*bls.VerKey, len(signKeys))
	pubKeys := make([]consensus.PublicKey, len(signKeys))
	ecdsaKeys := make([]*ecdsa.PrivateKey, len(signKeys))
	signKeysByAddress := make(map[string]*bls.SignKey)
	for i, signKey := range signKeys {
		ecdsaKey, _ := crypto.GenerateKey()
		verKeys[i] = signKey.MustVerKey()
		pubKeys[i] = consensus.BlsKeyToPublicKey(verKeys[i])
		ecdsaKeys[i] = ecdsaKey
		signKeysByAddress[consensus.BlsVerKeyToAddress(verKeys[i].Bytes()).String()] = signKey

	}

	return &testSet{
		SignKeys:          signKeys,
		VerKeys:           verKeys,
		PubKeys:           pubKeys,
		EcdsaKeys:         ecdsaKeys,
		SignKeysByAddress: signKeysByAddress,
	}
}

func groupFromTestSet(t *testing.T, set *testSet) *consensus.Group {
	members := make([]*consensus.RemoteNode, len(set.SignKeys))
	for i := range set.SignKeys {
		rn := consensus.NewRemoteNode(consensus.BlsKeyToPublicKey(set.VerKeys[i]), consensus.EcdsaToPublicKey(&set.EcdsaKeys[i].PublicKey))
		members[i] = rn
	}

	return consensus.NewGroup(members)
}

func blsKeys(size int) []*bls.SignKey {
	keys := make([]*bls.SignKey, size)
	for i := 0; i < size; i++ {
		keys[i] = bls.MustNewSignKey()
	}
	return keys
}

func generateTestGossipGroup(t *testing.T, size int, latency int) []*Gossiper {
	ts := newTestSet(t, size)
	group := groupFromTestSet(t, ts)

	system := NewInMemoryHandlerSystem(latency)

	gossipers := make([]*Gossiper, size)

	for i := 0; i < size; i++ {
		stor := storage.NewMemStorage()

		member := group.SortedMembers[i]

		gossiper := NewGossiper(&GossiperOpts{
			SignKey:            ts.SignKeysByAddress[member.Id],
			Storage:            stor,
			StateHandler:       simpleHandler,
			Group:              group,
			MessageHandler:     system.NewHandler(crypto.ToECDSAPub(member.DstKey.PublicKey)),
			TimeBetweenGossips: 200,
			NumberOfGossips:    3,
		})
		gossipers[i] = gossiper
	}

	return gossipers
}

func TestGossiper_HandleGossipRequest(t *testing.T) {
	gossipers := generateTestGossipGroup(t, 1, 0)

	message := &GossipMessage{
		ObjectId:    []byte("obj"),
		Transaction: []byte("trans"),
	}

	req, err := network.BuildRequest(MessageType_Gossip, message)
	assert.Nil(t, err)

	resp, err := gossipers[0].HandleGossipRequest(context.TODO(), *req)
	assert.Nil(t, err)

	gossipResp := &GossipMessage{}
	err = cbornode.DecodeInto(resp.Payload, gossipResp)
	assert.Nil(t, err)

	assert.Len(t, gossipResp.Signatures, 1)
}

func TestGossiper_DoOneGossip(t *testing.T) {
	gossipers := generateTestGossipGroup(t, 2, 0)

	message := &GossipMessage{
		ObjectId:    []byte("obj"),
		Transaction: []byte("trans"),
	}

	req, err := network.BuildRequest(MessageType_Gossip, message)
	assert.Nil(t, err)

	resp, err := gossipers[0].HandleGossipRequest(context.TODO(), *req)
	assert.Nil(t, err)

	gossipResp := &GossipMessage{}
	err = cbornode.DecodeInto(resp.Payload, gossipResp)
	assert.Nil(t, err)

	assert.Len(t, gossipResp.Signatures, 1)

	err = gossipers[0].DoOneGossip(gossipers[1].Group.SortedMembers[1].DstKey, message.Id())
	assert.Nil(t, err)

	// The original gossiper should have added the other gossiper
	sigs, err := gossipers[0].savedSignaturesFor(context.Background(), message.Id())
	assert.Nil(t, err)
	assert.Len(t, sigs, 2)

	// The new gossiper should have both its own and the gossiped signature
	sigs, err = gossipers[1].savedSignaturesFor(context.Background(), message.Id())
	assert.Nil(t, err)
	assert.Len(t, sigs, 2)
}

func TestGossiper_DoOneGossipRound(t *testing.T) {
	//log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(log.LvlError), log.StreamHandler(os.Stderr, log.TerminalFormat(true))))

	gossipers := generateTestGossipGroup(t, 3, 0)

	message := &GossipMessage{
		ObjectId:    []byte("obj"),
		Transaction: []byte("trans"),
	}

	req, err := network.BuildRequest(MessageType_Gossip, message)
	assert.Nil(t, err)

	resp, err := gossipers[0].HandleGossipRequest(context.Background(), *req)
	assert.Nil(t, err)

	gossipResp := &GossipMessage{}
	err = cbornode.DecodeInto(resp.Payload, gossipResp)
	assert.Nil(t, err)

	assert.Len(t, gossipResp.Signatures, 1)

	err = gossipers[0].DoOneGossipRound(message.Id())
	assert.Nil(t, err)

	// The original gossiper should have added the other gossiper
	sigs, err := gossipers[0].savedSignaturesFor(context.Background(), message.Id())
	assert.Nil(t, err)
	assert.Len(t, sigs, 3)

	// The new gossiper should have both its own and the gossiped signature
	sigs, err = gossipers[1].savedSignaturesFor(context.Background(), message.Id())
	assert.Nil(t, err)
	assert.Len(t, sigs, 2)
}
