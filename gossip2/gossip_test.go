package gossip2

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"log"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/p2p"
	"github.com/quorumcontrol/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newBootstrapHost(ctx context.Context, t *testing.T) *p2p.Host {
	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	host, err := p2p.NewHost(ctx, key, p2p.GetRandomUnusedPort())

	require.Nil(t, err)
	require.NotNil(t, host)

	return host
}

func bootstrapAddresses(bootstrapHost *p2p.Host) []string {
	addresses := bootstrapHost.Addresses()
	for _, addr := range addresses {
		addrStr := addr.String()
		if strings.Contains(addrStr, "127.0.0.1") {
			return []string{addrStr}
		}
	}
	return nil
}

type testSet struct {
	SignKeys          []*bls.SignKey
	VerKeys           []*bls.VerKey
	EcdsaKeys         []*ecdsa.PrivateKey
	PubKeys           []consensus.PublicKey
	SignKeysByAddress map[string]*bls.SignKey
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

func groupFromTestSet(t *testing.T, set *testSet) *consensus.NotaryGroup {
	members := make([]*consensus.RemoteNode, len(set.SignKeys))
	for i := range set.SignKeys {
		rn := consensus.NewRemoteNode(consensus.BlsKeyToPublicKey(set.VerKeys[i]), consensus.EcdsaToPublicKey(&set.EcdsaKeys[i].PublicKey))
		members[i] = rn
	}

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	group := consensus.NewNotaryGroup("notarygroupid", nodeStore)
	err := group.CreateGenesisState(group.RoundAt(time.Now()), members...)
	require.Nil(t, err)
	return group
}

func blsKeys(size int) []*bls.SignKey {
	keys := make([]*bls.SignKey, size)
	for i := 0; i < size; i++ {
		keys[i] = bls.MustNewSignKey()
	}
	return keys
}

func TestGossip(t *testing.T) {
	groupSize := 5
	ts := newTestSet(t, groupSize)
	group := groupFromTestSet(t, ts)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bootstrap := newBootstrapHost(ctx, t)

	gossipNodes := make([]*GossipNode, groupSize)

	for i := 0; i < groupSize; i++ {
		host, err := p2p.NewHost(ctx, ts.EcdsaKeys[i], p2p.GetRandomUnusedPort())
		require.Nil(t, err)
		host.Bootstrap(bootstrapAddresses(bootstrap))
		path := "test/badger/" + strconv.Itoa(i)
		os.MkdirAll(path, 0755)
		defer func() {
			os.RemoveAll(path)
		}()
		storage := NewBadgerStorage(path)
		gossipNodes[i] = NewGossipNode(ts.EcdsaKeys[i], host, storage)
		gossipNodes[i].Group = group
		gossipNodes[i].SignKey = ts.SignKeys[i]
	}

	transaction1 := Transaction{
		ObjectID:    []byte("himynameisalongobjectidthatwillhavemorethan64bits"),
		PreviousTip: []byte(""),
		NewTip:      []byte("zdpuAs5LQAGsXbGTF3DbfGVkRw4sWJd4MzbbigtJ4zE6NNJrr"),
		Payload:     []byte("thisisthepayload"),
	}
	log.Printf("gossipNode0 is %s", gossipNodes[0].ID())
	transaction1ID, err := gossipNodes[0].InitiateTransaction(transaction1)
	require.Nil(t, err)

	transaction2 := Transaction{
		ObjectID:    []byte("DIFFERENTOBJhimynameisalongobjectidthatwillhavemorethan64bits"),
		PreviousTip: []byte(""),
		NewTip:      []byte("zdpuAx6tV9jpLEwhvGB8bdYjUvkomzdHf7ze6ckdNqJur7JBr"),
		Payload:     []byte("thisisthepayload"),
	}
	var transaction2ID []byte
	for i := 1; i < len(gossipNodes)-1; i++ {
		transaction2ID, err = gossipNodes[i].InitiateTransaction(transaction2)
		require.Nil(t, err)
	}

	time.Sleep(2 * time.Second)
	for i := 0; i < 10; i++ {
		err = gossipNodes[0].DoSync()
		require.Nil(t, err)
		log.Print("")
		log.Print("+++++++++++++++++++++++++++")
		log.Printf("ITEREATION %v done", i)
		log.Print("+++++++++++++++++++++++++++")
	}
	time.Sleep(3 * time.Second)

	val, err := gossipNodes[0].Storage.Get(transaction1ID)
	require.Nil(t, err)
	encodedTransaction1, err := transaction1.MarshalMsg(nil)
	require.Nil(t, err)
	assert.Equal(t, val, encodedTransaction1)

	val, err = gossipNodes[0].Storage.Get(transaction2ID)
	require.Nil(t, err)
	encodedTransaction2, err := transaction2.MarshalMsg(nil)
	require.Nil(t, err)
	assert.Equal(t, val, encodedTransaction2)

	matchedOne := false
	for i := 1; i < len(gossipNodes)-1; i++ {
		otherVal, err := gossipNodes[i].Storage.Get(transaction1ID)
		require.Nil(t, err)
		if bytes.Equal(otherVal, encodedTransaction1) {
			matchedOne = true
		}
	}
	assert.True(t, matchedOne)
}

func TestTransactionIDFromSignatureKey(t *testing.T) {
	transaction := Transaction{
		ObjectID:    []byte("himynameisalongobjectidthatwillhavemorethan64bits"),
		PreviousTip: []byte(""),
		NewTip:      []byte("zdpuAs5LQAGsXbGTF3DbfGVkRw4sWJd4MzbbigtJ4zE6NNJrr"),
		Payload:     []byte("thisisthepayload"),
	}
	signature := Signature{
		TransactionID: transaction.ID(),
	}

	assert.Equal(t, transaction.StoredID(), transactionIDFromSignatureKey(signature.StoredID(transaction.ToConflictSet().ID())))
}
