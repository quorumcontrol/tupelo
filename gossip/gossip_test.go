package gossip

import (
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
	SignKeys  []*bls.SignKey
	VerKeys   []*bls.VerKey
	EcdsaKeys []*ecdsa.PrivateKey
	PubKeys   []consensus.PublicKey
}

// This is the simplest possible state handler that just always returns the transaction as the nextState
func simpleHandler(_ []byte, transaction []byte) (nextState []byte, err error) {
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
	return &InMemoryHandler{
		Id:     crypto.PubkeyToAddress(*dst).String(),
		System: imhs,
	}
}

type InMemoryHandler struct {
	Id      string
	System  *InMemoryHandlerSystem
	Mapping map[string]network.HandlerFunc
}

func (imh *InMemoryHandler) DoRequest(dst *ecdsa.PublicKey, req *network.Request) (chan *network.Response, error) {
	respChan := make(chan *network.Response)

	go func() {
		handler := imh.System.Handlers[crypto.PubkeyToAddress(*dst).String()]
		resp, err := handler.Mapping[req.Type](*req)
		if err != nil {
			log.Error("error handling request: %v", err)
		}
		time.Sleep(time.Duration(imh.System.ArtificialLatency) * time.Millisecond)
		respChan <- resp
	}()

	return respChan, nil
}

func (imh *InMemoryHandler) AssignHandler(requestType string, handlerFunc network.HandlerFunc) error {
	if imh.Mapping == nil {
		imh.Mapping = make(map[string]network.HandlerFunc)
	}

	imh.Mapping[requestType] = handlerFunc
	return nil
}

func newTestSet(t *testing.T) *testSet {
	signKeys := blsKeys(5)
	verKeys := make([]*bls.VerKey, len(signKeys))
	pubKeys := make([]consensus.PublicKey, len(signKeys))
	ecdsaKeys := make([]*ecdsa.PrivateKey, len(signKeys))
	for i, signKey := range signKeys {
		ecdsaKey, _ := crypto.GenerateKey()
		verKeys[i] = signKey.MustVerKey()
		pubKeys[i] = consensus.BlsKeyToPublicKey(verKeys[i])
		ecdsaKeys[i] = ecdsaKey
	}

	return &testSet{
		SignKeys:  signKeys,
		VerKeys:   verKeys,
		PubKeys:   pubKeys,
		EcdsaKeys: ecdsaKeys,
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

func generateTestGossipGroup(t *testing.T) []*Gossiper {
	ts := newTestSet(t)
	group := groupFromTestSet(t, ts)

	stor := storage.NewMemStorage()

	system := NewInMemoryHandlerSystem(0)

	gossiper := &Gossiper{
		Id:             group.SortedMembers[0].Id,
		SignKey:        ts.SignKeys[0],
		Storage:        stor,
		StateHandler:   simpleHandler,
		Group:          group,
		MessageHandler: system.NewHandler(crypto.ToECDSAPub(group.SortedMembers[0].DstKey.PublicKey)),
	}

	gossiper.Initialize()

	return []*Gossiper{gossiper}
}

func TestGossiper_HandleGossipRequest(t *testing.T) {
	gossipers := generateTestGossipGroup(t)

	message := &GossipMessage{
		ObjectId:    []byte("obj"),
		Transaction: []byte("trans"),
	}

	req, err := network.BuildRequest(MessageType_Gossip, message)
	assert.Nil(t, err)

	resp, err := gossipers[0].HandleGossipRequest(*req)
	assert.Nil(t, err)

	gossipResp := &GossipMessage{}
	err = cbornode.DecodeInto(resp.Payload, gossipResp)
	assert.Nil(t, err)

	assert.Len(t, gossipResp.Signatures, 1)

}
