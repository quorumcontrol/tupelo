package gossip2

import (
	"context"
	"crypto/ecdsa"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	logging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/p2p"
	"github.com/quorumcontrol/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testStoragePath = "../.tmp/test/"

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
		ecdsaKey, err := crypto.GenerateKey()
		if err != nil {
			t.Fatalf("error generating key: %v", err)
		}
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

func randBytes(length int) []byte {
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		panic("couldn't generate random bytes")
	}
	return b
}

func TestGossip(t *testing.T) {
	logging.SetLogLevel("gossip", "ERROR")
	groupSize := 40
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
		path := testStoragePath + "badger/" + strconv.Itoa(i)
		os.RemoveAll(path)
		os.MkdirAll(path, 0755)
		defer func() {
			os.RemoveAll(path)
		}()
		storage := NewBadgerStorage(path)
		gossipNodes[i] = NewGossipNode(ts.EcdsaKeys[i], ts.SignKeys[i], host, storage)
		gossipNodes[i].Group = group
	}

	transaction1 := Transaction{
		ObjectID:    []byte("himynameisalongobjectidthatwillhavemorethan64bits"),
		PreviousTip: []byte(""),
		NewTip:      []byte("zdpuAs5LQAGsXbGTF3DbfGVkRw4sWJd4MzbbigtJ4zE6NNJrr"),
		Payload:     []byte("thisisthepayload"),
	}
	log.Debugf("gossipNode0 is %s", gossipNodes[0].ID())

	for i := 0; i < groupSize; i++ {
		defer gossipNodes[i].Stop()
		go gossipNodes[i].Start()
	}

	// This bit of commented out code will run the CPU profiler
	// f, ferr := os.Create("../gossip.prof")
	// if ferr != nil {
	// 	t.Fatal(ferr)
	// }
	// pprof.StartCPUProfile(f)
	// defer pprof.StopCPUProfile()

	for i := 0; i < 300; i++ {
		_, err := gossipNodes[rand.Intn(len(gossipNodes))].InitiateTransaction(Transaction{
			ObjectID:    randBytes(32),
			PreviousTip: []byte(""),
			NewTip:      randBytes(49),
			Payload:     randBytes(rand.Intn(400) + 100),
		})
		if err != nil {
			t.Fatalf("error sending transaction: %v", err)
		}
	}

	start := time.Now()
	_, err := gossipNodes[0].InitiateTransaction(transaction1)
	require.Nil(t, err)

	var stop time.Time
	for {
		if (time.Now().Sub(start)) > (60 * time.Second) {
			t.Fatal("timed out looking for done function")
			break
		}
		exists, err := gossipNodes[0].Storage.Exists(transaction1.ToConflictSet().DoneID())
		require.Nil(t, err)
		if exists {
			stop = time.Now()
			break
		}
	}

	t.Logf("Confirmation took %f seconds\n", stop.Sub(start).Seconds())
	assert.True(t, stop.Sub(start) < 10*time.Second)

	for i := 0; i < 1; i++ {
		exists, err := gossipNodes[i].Storage.Exists(transaction1.ToConflictSet().DoneID())
		require.Nil(t, err)
		assert.True(t, exists)
	}

	csq := ConflictSetQuery{Key: transaction1.ToConflictSet().ID()}
	csqBytes, err := csq.MarshalMsg(nil)
	require.Nil(t, err)

	csqrBytes, err := gossipNodes[1].Host.SendAndReceive(&gossipNodes[0].Key.PublicKey, IsDoneProtocol, csqBytes)
	require.Nil(t, err)
	csqr := ConflictSetQueryResponse{}
	_, err = csqr.UnmarshalMsg(csqrBytes)
	require.Nil(t, err)
	assert.True(t, csqr.Done)

	var totalIn int64
	var totalOut int64
	var totalReceiveSync int64
	var totalAttemptSync int64
	for _, gn := range gossipNodes {
		totalIn += gn.Host.Reporter.GetBandwidthTotals().TotalIn
		totalOut += gn.Host.Reporter.GetBandwidthTotals().TotalOut
		totalReceiveSync += int64(atomic.LoadUint64(&gn.debugReceiveSync))
		totalAttemptSync += int64(atomic.LoadUint64(&gn.debugAttemptSync))
	}
	t.Logf(
		`Bandwidth In: %d, Bandwidth Out: %d,
Average In %d, Average Out %d
Total Receive Syncs: %d, Average Receive Sync: %d
Total Attempted Syncs: %d, Average Attepted Syncs: %d`,
		totalIn,
		totalOut,
		totalIn/int64(len(gossipNodes)),
		totalOut/int64(len(gossipNodes)),
		totalReceiveSync,
		totalReceiveSync/int64(len(gossipNodes)),
		totalAttemptSync,
		totalAttemptSync/int64(len(gossipNodes)),
	)

}

func TestDeadlockTransactionGossip(t *testing.T) {
	logging.SetLogLevel("gossip", "DEBUG")
	groupSize := 2
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
		path := testStoragePath + "badger/" + strconv.Itoa(i)
		os.RemoveAll(path)
		os.MkdirAll(path, 0755)
		defer func() {
			os.RemoveAll(path)
		}()
		storage := NewBadgerStorage(path)
		gossipNodes[i] = NewGossipNode(ts.EcdsaKeys[i], ts.SignKeys[i], host, storage)
		gossipNodes[i].Group = group
	}

	objectID := randBytes(32)

	transaction1 := Transaction{
		ObjectID:    objectID,
		PreviousTip: []byte(""),
		NewTip:      randBytes(49),
		Payload:     randBytes(rand.Intn(400) + 100),
	}

	_, err := gossipNodes[0].InitiateTransaction(transaction1)
	require.Nil(t, err)

	transaction2 := Transaction{
		ObjectID:    objectID,
		PreviousTip: []byte(""),
		NewTip:      randBytes(49),
		Payload:     randBytes(rand.Intn(400) + 100),
	}
	_, err = gossipNodes[1].InitiateTransaction(transaction2)
	require.Nil(t, err)

	for i := 0; i < groupSize; i++ {
		defer gossipNodes[i].Stop()
		go gossipNodes[i].Start()
	}

	var exists1 bool
	var exists2 bool

	start := time.Now()
	for {
		if (time.Now().Sub(start)) > (60 * time.Second) {
			t.Fatal("timed out looking for done function")
			break
		}
		exists1, err = gossipNodes[0].Storage.Exists(transaction2.ToConflictSet().DoneID())
		require.Nil(t, err)
		exists2, err = gossipNodes[1].Storage.Exists(transaction1.ToConflictSet().DoneID())
		require.Nil(t, err)
		time.Sleep(1 * time.Second)
		if exists1 && exists2 {
			break
		}
	}

	// TODO actually check tips match here once we store tips
	assert.True(t, exists1)
	assert.True(t, exists2)
}
