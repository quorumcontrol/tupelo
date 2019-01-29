package actors

import (
	"context"
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/testhelpers"
	"github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTupeloSystem(ctx context.Context, testSet *testnotarygroup.TestSet) (*types.NotaryGroup, error) {
	ng := types.NewNotaryGroup("testnotary")
	for i, signKey := range testSet.SignKeys {
		sk := signKey
		signer := types.NewLocalSigner(testSet.PubKeys[i].ToEcdsaPub(), sk)
		syncer, err := actor.SpawnNamed(NewTupeloNodeProps(&TupeloConfig{
			Self:              signer,
			NotaryGroup:       ng,
			CommitStore:       storage.NewMemStorage(),
			CurrentStateStore: storage.NewMemStorage(),
		}), "tupelo-"+signer.ID)
		if err != nil {
			return nil, fmt.Errorf("error spawning: %v", err)
		}
		signer.Actor = syncer
		go func() {
			<-ctx.Done()
			syncer.Poison()
		}()
		ng.AddSigner(signer)
	}
	return ng, nil
}
func TestCommits(t *testing.T) {
	numMembers := 20
	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		middleware.Log.Infow("---- tests over ----")
		cancel()
	}()
	ts := testnotarygroup.NewTestSet(t, numMembers)

	system, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)

	syncers := system.AllSigners()
	require.Len(t, system.Signers, numMembers)
	t.Logf("syncer 0 id: %s", syncers[0].ID)

	// for i := 0; i < 100; i++ {
	// 	trans := testhelpers.NewValidTransaction(t)
	// 	bits, err := trans.MarshalMsg(nil)
	// 	require.Nil(t, err)
	// 	key := crypto.Keccak256(bits)
	// 	syncers[rand.Intn(len(syncers))].Actor.Tell(&messages.Store{
	// 		Key:   key,
	// 		Value: bits,
	// 	})
	// 	syncers[rand.Intn(len(syncers))].Actor.Tell(&messages.Store{
	// 		Key:   key,
	// 		Value: bits,
	// 	})
	// 	syncers[rand.Intn(len(syncers))].Actor.Tell(&messages.Store{
	// 		Key:   key,
	// 		Value: bits,
	// 	})
	// 	if err != nil {
	// 		t.Fatalf("error sending transaction: %v", err)
	// 	}
	// }

	for _, s := range syncers {
		s.Actor.Tell(&messages.StartGossip{})
	}

	// t.Run("removes bad transactions", func(t *testing.T) {
	// 	trans := testhelpers.NewValidTransaction(t)
	// 	bits, err := trans.MarshalMsg(nil)
	// 	require.Nil(t, err)
	// 	bits = append([]byte{byte(1)}, bits...) // append a bad byte
	// 	id := crypto.Keccak256(bits)
	// 	syncers[0].Actor.Tell(&messages.Store{
	// 		Key:   id,
	// 		Value: bits,
	// 	})
	// 	ret, err := syncers[0].Actor.RequestFuture(&messages.Get{Key: id}, 5*time.Second).Result()
	// 	require.Nil(t, err)
	// 	assert.Equal(t, ret, bits)

	// 	// wait for it to get removed in the sync
	// 	time.Sleep(100 * time.Millisecond)

	// 	ret, err = syncers[0].Actor.RequestFuture(&messages.Get{Key: id}, 5*time.Second).Result()
	// 	require.Nil(t, err)
	// 	assert.Empty(t, ret)
	// })

	// t.Run("commits a good transaction", func(t *testing.T) {
	// 	trans := testhelpers.NewValidTransaction(t)
	// 	bits, err := trans.MarshalMsg(nil)
	// 	require.Nil(t, err)
	// 	key := crypto.Keccak256(bits)

	// 	fut := actor.NewFuture(20 * time.Second)

	// 	syncer := syncers[rand.Intn(len(syncers))].Actor

	// 	syncer.Request(&messages.TipSubscription{
	// 		ObjectID: trans.ObjectID,
	// 	}, fut.PID())

	// 	syncer.Tell(&messages.Store{
	// 		Key:   key,
	// 		Value: bits,
	// 	})

	// 	resp, err := fut.Result()
	// 	require.Nil(t, err)
	// 	assert.Equal(t, resp.(*messages.CurrentState).Signature.NewTip, trans.NewTip)
	// })

	t.Run("triple set data", func(t *testing.T) {
		treeKey, err := crypto.GenerateKey()
		require.Nil(t, err)

		path := "/path/to/somewhere"
		value := "value"

		sw := safewrap.SafeWrap{}

		treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())

		unsignedBlock := &chaintree.BlockWithHeaders{
			Block: chaintree.Block{
				PreviousTip: "",
				Transactions: []*chaintree.Transaction{
					{
						Type: "SET_DATA",
						Payload: map[string]string{
							"path":  path,
							"value": value,
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
		trans := messages.Transaction{
			PreviousTip: emptyTip.Bytes(),
			NewTip:      testTree.Dag.Tip.Bytes(),
			Payload:     sw.WrapObject(req).RawData(),
			ObjectID:    []byte(treeDID),
		}

		bits, err := trans.MarshalMsg(nil)
		require.Nil(t, err)
		key := crypto.Keccak256(bits)

		fut := actor.NewFuture(20 * time.Second)

		syncer := syncers[rand.Intn(len(syncers))].Actor

		syncer.Request(&messages.TipSubscription{
			ObjectID: trans.ObjectID,
		}, fut.PID())

		syncer.Tell(&messages.Store{
			Key:   key,
			Value: bits,
		})

		resp, err := fut.Result()
		require.Nil(t, err)
		assert.Equal(t, resp.(*messages.CurrentState).Signature.NewTip, trans.NewTip)

		// do it again

		unsignedBlock2 := &chaintree.BlockWithHeaders{
			Block: chaintree.Block{
				PreviousTip: string(trans.NewTip),
				Transactions: []*chaintree.Transaction{
					{
						Type: "SET_DATA",
						Payload: map[string]string{
							"path":  path,
							"value": "different value",
						},
					},
				},
			},
		}

		blockWithHeaders2, err := consensus.SignBlock(unsignedBlock2, treeKey)
		require.Nil(t, err)

		oldTip := testTree.Dag.Tip

		testTree.ProcessBlock(blockWithHeaders)
		nodes2 := dagToByteNodes(t, testTree.Dag)

		req2 := &consensus.AddBlockRequest{
			Nodes:    nodes2,
			Tip:      &oldTip,
			NewBlock: blockWithHeaders2,
		}
		trans2 := messages.Transaction{
			PreviousTip: oldTip.Bytes(),
			NewTip:      testTree.Dag.Tip.Bytes(),
			Payload:     sw.WrapObject(req2).RawData(),
			ObjectID:    []byte(treeDID),
		}

		bits2, err := trans2.MarshalMsg(nil)
		require.Nil(t, err)
		key2 := crypto.Keccak256(bits2)

		fut2 := actor.NewFuture(10 * time.Second)

		syncer2 := syncers[rand.Intn(len(syncers))].Actor

		syncer2.Request(&messages.TipSubscription{
			ObjectID: trans2.ObjectID,
		}, fut2.PID())

		syncer2.Tell(&messages.Store{
			Key:   key2,
			Value: bits2,
		})

		resp2, err := fut2.Result()
		require.Nil(t, err)
		assert.Equal(t, resp2.(*messages.CurrentState).Signature.NewTip, trans2.NewTip)

	})

	// t.Run("reaches another node", func(t *testing.T) {
	// 	trans := testhelpers.NewValidTransaction(t)
	// 	bits, err := trans.MarshalMsg(nil)
	// 	require.Nil(t, err)
	// 	key := crypto.Keccak256(bits)

	// 	fut := actor.NewFuture(20 * time.Second)

	// 	syncers[1].Actor.Request(&messages.TipSubscription{
	// 		ObjectID: trans.ObjectID,
	// 	}, fut.PID())

	// 	start := time.Now()
	// 	syncers[0].Actor.Tell(&messages.Store{
	// 		Key:   key,
	// 		Value: bits,
	// 	})

	// 	resp, err := fut.Result()
	// 	require.Nil(t, err)
	// 	assert.Equal(t, resp.(*messages.CurrentState).Signature.NewTip, trans.NewTip)

	// 	stop := time.Now()
	// 	t.Logf("Confirmation took %f seconds\n", stop.Sub(start).Seconds())
	// })

}

func TestTupeloMemStorage(t *testing.T) {
	numMembers := 3
	ts := testnotarygroup.NewTestSet(t, numMembers)

	ctx, cancel := context.WithCancel(context.Background())
	defer func() {
		middleware.Log.Infow("---- tests over ----")
		cancel()
	}()
	system, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)

	require.Len(t, system.Signers, numMembers)
	syncer := system.AllSigners()[0].Actor

	trans := testhelpers.NewValidTransaction(t)
	bits, err := trans.MarshalMsg(nil)
	require.Nil(t, err)
	id := crypto.Keccak256(bits)

	key := id
	value := bits

	syncer.Tell(&messages.Store{
		Key:   key,
		Value: value,
	})

	time.Sleep(10 * time.Millisecond)

	val, err := syncer.RequestFuture(&messages.Get{Key: key}, 1*time.Second).Result()
	require.Nil(t, err)
	require.Equal(t, value, val)
}
