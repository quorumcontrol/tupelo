// +build integration

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

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	logging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/bls"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tinylib/msgp/msgp"
)

func newBootstrapHost(ctx context.Context, t *testing.T) *p2p.Host {
	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	host, err := p2p.NewHost(ctx, key, 0)

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

func newValidTransaction(t *testing.T) Transaction {
	sw := safewrap.SafeWrap{}
	treeKey, err := crypto.GenerateKey()
	require.Nil(t, err)

	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]string{
						"path":  "down/in/the/thing",
						"value": "hi",
					},
				},
			},
		},
	}

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	emptyTree := consensus.NewEmptyTree(treeDID, nodeStore)
	emptyTip := emptyTree.Tip
	testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)
	require.Nil(t, err)

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	require.Nil(t, err)

	testTree.ProcessBlock(blockWithHeaders)
	nodes := dagToByteNodes(t, emptyTree)

	req := &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      &emptyTree.Tip,
		NewBlock: blockWithHeaders,
	}
	return Transaction{
		PreviousTip: emptyTip.Bytes(),
		NewTip:      testTree.Dag.Tip.Bytes(),
		Payload:     sw.WrapObject(req).RawData(),
		ObjectID:    []byte(treeDID),
	}
}

func NewTestCluster(t *testing.T, groupSize int, ctx context.Context) []*GossipNode {
	gossipNodes := make([]*GossipNode, groupSize)
	ts := testnotarygroup.NewTestSet(t, groupSize)
	group := testnotarygroup.GroupFromTestSet(t, ts)
	bootstrap := testnotarygroup.NewBootstrapHost(ctx, t)

	pathsToCleanup := make([]string, groupSize, groupSize)

	for i := 0; i < groupSize; i++ {
		host, err := p2p.NewHost(ctx, ts.EcdsaKeys[i], 0)
		require.Nil(t, err)
		host.Bootstrap(bootstrapAddresses(bootstrap))
		path := testStoragePath + "badger/" + strconv.Itoa(i)
		os.RemoveAll(path)
		os.MkdirAll(path, 0755)
		pathsToCleanup[i] = path
		storage := NewBadgerStorage(path)
		gossipNodes[i] = NewGossipNode(ts.EcdsaKeys[i], ts.SignKeys[i], host, storage)
		gossipNodes[i].Group = group
	}

	go func(ctx context.Context, pathsToCleanup []string) {
		<-ctx.Done()
		for _, path := range pathsToCleanup {
			os.RemoveAll(path)
		}
	}(ctx, pathsToCleanup)

	return gossipNodes
}

func TestGossip(t *testing.T) {
	logging.SetLogLevel("gossip", "ERROR")
	groupSize := 20

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	gossipNodes := NewTestCluster(t, groupSize, ctx)

	transaction1 := newValidTransaction(t)
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

	for i := 0; i < 100; i++ {
		_, err := gossipNodes[rand.Intn(len(gossipNodes))].InitiateTransaction(newValidTransaction(t))
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
		time.Sleep(100 * time.Millisecond)
	}

	t.Logf("Confirmation took %f seconds\n", stop.Sub(start).Seconds())
	assert.True(t, stop.Sub(start) < 30*time.Second)

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

	// The done could still be processing, so need to wait before we actually get a tip

	// tipQuery := TipQuery{ObjectID: transaction1.ObjectID}
	// tipQueryBytes, err := tipQuery.MarshalMsg(nil)
	// require.Nil(t, err)

	// stateBytes, err := gossipNodes[1].Host.SendAndReceive(&gossipNodes[0].Key.PublicKey, TipProtocol, tipQueryBytes)
	// require.Nil(t, err)
	// var currState CurrentState
	// _, err = currState.UnmarshalMsg(stateBytes)
	// require.Nil(t, err)
	// assert.Equal(t, transaction1.NewTip, currState.Tip)

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
	logging.SetLogLevel("gossip", "ERROR")
	groupSize := 2
	ts := newTestSet(t, groupSize)
	group := groupFromTestSet(t, ts)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bootstrap := newBootstrapHost(ctx, t)

	gossipNodes := make([]*GossipNode, groupSize)

	for i := 0; i < groupSize; i++ {
		host, err := p2p.NewHost(ctx, ts.EcdsaKeys[i], 0)
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

	sw := safewrap.SafeWrap{}
	// NOTE: must use this key to make this test deterministic
	// because the ObjectID changing, changes the transaction hashes
	// and so it's non deterministic which is lower
	keyBytes, err := hexutil.Decode("0xf9c0b741e7c065ea4fe4fde335c4ee575141db93236e3d86bb1c9ae6ccddf6f1")
	require.Nil(t, err)
	treeKey, err := crypto.ToECDSA(keyBytes)
	require.Nil(t, err)

	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]string{
						"path":  "down/in/the/thing",
						"value": "hi",
					},
				},
			},
		},
	}

	conflictingBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]string{
						"path":  "down/in/the/thing",
						"value": "DIFFERENTDIFFERENT",
					},
				},
			},
		},
	}

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	emptyTree := consensus.NewEmptyTree(treeDID, nodeStore)
	emptyTip := emptyTree.Tip
	testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)
	require.Nil(t, err)

	conflictingTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)
	require.Nil(t, err)

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	require.Nil(t, err)

	conflictingBlockWithHeaders, err := consensus.SignBlock(conflictingBlock, treeKey)
	require.Nil(t, err)

	testTree.ProcessBlock(blockWithHeaders)
	nodes := dagToByteNodes(t, emptyTree)

	req := &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      &emptyTree.Tip,
		NewBlock: blockWithHeaders,
	}
	transaction1 := Transaction{
		PreviousTip: emptyTip.Bytes(),
		NewTip:      testTree.Dag.Tip.Bytes(),
		Payload:     sw.WrapObject(req).RawData(),
		ObjectID:    []byte(treeDID),
	}

	conflictingTree.ProcessBlock(conflictingBlockWithHeaders)
	nodes = dagToByteNodes(t, emptyTree)

	req2 := &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      &emptyTree.Tip,
		NewBlock: conflictingBlockWithHeaders,
	}
	transaction2 := Transaction{
		PreviousTip: emptyTip.Bytes(),
		NewTip:      conflictingTree.Dag.Tip.Bytes(),
		Payload:     sw.WrapObject(req2).RawData(),
		ObjectID:    []byte(treeDID),
	}

	require.True(t, string(transaction2.ID()) < string(transaction1.ID()))

	_, err = gossipNodes[0].InitiateTransaction(transaction2)
	require.Nil(t, err)

	_, err = gossipNodes[0].InitiateTransaction(transaction1)
	require.Nil(t, err)

	_, err = gossipNodes[1].InitiateTransaction(transaction1)
	require.Nil(t, err)

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
		exists1, err = gossipNodes[1].Storage.Exists(transaction1.ToConflictSet().DoneID())
		require.Nil(t, err)
		exists2, err = gossipNodes[0].Storage.Exists(transaction2.ToConflictSet().DoneID())
		require.Nil(t, err)
		time.Sleep(200 * time.Millisecond)
		if exists1 && exists2 {
			break
		}
	}

	time.Sleep(500 * time.Millisecond)

	currState, err := gossipNodes[0].getCurrentState(transaction1.ObjectID)
	require.Nil(t, err)

	assert.Equal(t, transaction2.NewTip, currState.Tip)
}

func TestSubscription(t *testing.T) {
	logging.SetLogLevel("gossip", "ERROR")
	groupSize := 3
	ts := newTestSet(t, groupSize)
	group := groupFromTestSet(t, ts)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bootstrap := newBootstrapHost(ctx, t)

	gossipNodes := make([]*GossipNode, groupSize)

	for i := 0; i < groupSize; i++ {
		host, err := p2p.NewHost(ctx, ts.EcdsaKeys[i], 0)
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
		defer gossipNodes[i].Stop()
		go gossipNodes[i].Start()
	}

	sw := safewrap.SafeWrap{}
	// NOTE: must use this key to make this test deterministic
	// because the ObjectID changing, changes the transaction hashes
	// and so it's non deterministic which is lower
	keyBytes, err := hexutil.Decode("0xf9c0b741e7c065ea4fe4fde335c4ee575141db93236e3d86bb1c9ae6ccddf6f1")
	require.Nil(t, err)
	treeKey, err := crypto.ToECDSA(keyBytes)
	require.Nil(t, err)
	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]string{
						"path":  "down/in/the/thing",
						"value": "hi",
					},
				},
			},
		},
	}

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	emptyTree := consensus.NewEmptyTree(treeDID, nodeStore)
	emptyTip := emptyTree.Tip
	testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)
	require.Nil(t, err)

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	require.Nil(t, err)
	testTree.ProcessBlock(blockWithHeaders)
	nodes := dagToByteNodes(t, emptyTree)

	req := &consensus.AddBlockRequest{
		Nodes:    nodes,
		Tip:      &emptyTree.Tip,
		NewBlock: blockWithHeaders,
	}
	transaction := Transaction{
		PreviousTip: emptyTip.Bytes(),
		NewTip:      testTree.Dag.Tip.Bytes(),
		Payload:     sw.WrapObject(req).RawData(),
		ObjectID:    []byte(treeDID),
	}

	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	_, err = gossipNodes[0].InitiateTransaction(transaction)
	require.Nil(t, err)

	subscriptionReq := &ChainTreeSubscriptionRequest{ObjectID: transaction.ObjectID}
	msg, err := subscriptionReq.MarshalMsg(nil)
	client, err := p2p.NewHost(ctx, key, 0)
	client.Bootstrap(bootstrapAddresses(bootstrap))
	require.Nil(t, err)

	ctx, cancel2 := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel2()
	stream, err := client.NewStream(ctx, &gossipNodes[0].Key.PublicKey, ChainTreeChangeProtocol)
	require.Nil(t, err)
	defer stream.Close()
	_, err = stream.Write(msg)
	require.Nil(t, err)

	reader := msgp.NewReader(stream)

	var resp ProtocolMessage
	err = resp.DecodeMsg(reader)
	require.Nil(t, err)

	_, err = FromProtocolMessage(&resp)
	require.Nil(t, err)

	var resp2 ProtocolMessage
	err = resp2.DecodeMsg(reader)
	require.Nil(t, err)

	currentState, err := FromProtocolMessage(&resp2)
	require.Nil(t, err)

	assert.Equal(t, transaction.NewTip, currentState.(*CurrentState).Tip)
}

func TestNodeRestartMaintainsIBF(t *testing.T) {
	logging.SetLogLevel("gossip", "ERROR")
	groupSize := 1
	ts := newTestSet(t, groupSize)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	host, err := p2p.NewHost(ctx, ts.EcdsaKeys[0], 0)
	require.Nil(t, err)
	path := testStoragePath + "badger/node1"
	os.RemoveAll(path)
	os.MkdirAll(path, 0755)
	defer os.RemoveAll(path)
	storage := NewBadgerStorage(path)
	node1 := NewGossipNode(ts.EcdsaKeys[0], ts.SignKeys[0], host, storage)
	node1.Add((&Signature{}).StoredID(randBytes(20)), randBytes(20))
	node1.Add((&Transaction{}).StoredID(), randBytes(20))
	node1.Add((&ConflictSet{}).DoneID(), randBytes(20))
	node2 := NewGossipNode(ts.EcdsaKeys[0], ts.SignKeys[0], host, storage)

	ibfSize := standardIBFSizes[0]
	newIBF := node1.IBFs[ibfSize].Subtract(node2.IBFs[ibfSize])
	results, err := newIBF.Decode()

	assert.Equal(t, len(results.LeftSet), 0)
	assert.Equal(t, len(results.RightSet), 0)
}
