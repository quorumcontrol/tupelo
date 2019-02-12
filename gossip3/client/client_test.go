package client

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	cid "github.com/ipfs/go-cid"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/quorumcontrol/tupelo/gossip3/actors"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/testhelpers"
	"github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClientSendTransaction(t *testing.T) {
	ts := testnotarygroup.NewTestSet(t, 3)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ng, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)

	client := New(ng)
	defer client.Stop()

	trans := testhelpers.NewValidTransaction(t)
	err = client.SendTransaction(ng.GetRandomSigner(), &trans)
	require.Nil(t, err)
}

func TestClientSubscribe(t *testing.T) {
	ts := testnotarygroup.NewTestSet(t, 3)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ng, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)

	for _, s := range ng.AllSigners() {
		s.Actor.Tell(&messages.StartGossip{})
	}

	trans := testhelpers.NewValidTransaction(t)
	client := New(ng)
	defer client.Stop()

	ch, err := client.Subscribe(ng.GetRandomSigner(), string(trans.ObjectID), 5*time.Second)
	require.Nil(t, err)

	err = client.SendTransaction(ng.GetRandomSigner(), &trans)
	require.Nil(t, err)

	resp := <-ch
	require.NotNil(t, ch)
	assert.Equal(t, resp.Signature.NewTip, trans.NewTip)
}

func TestPlayTransactions(t *testing.T) {
	l := middleware.Log.Named("TestPlayTransactions")
	ts := testnotarygroup.NewTestSet(t, 3)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ng, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)

	for _, s := range ng.AllSigners() {
		s.Actor.Tell(&messages.StartGossip{})
	}

	client := New(ng)
	defer client.Stop()

	treeKey, err := crypto.GenerateKey()
	require.Nil(t, err)
	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	chain, err := consensus.NewSignedChainTree(treeKey.PublicKey, nodeStore)

	var remoteTip cid.Cid
	if !chain.IsGenesis() {
		remoteTip = chain.Tip()
	}

	resp, err := client.PlayTransactions(chain, treeKey, &remoteTip, []*chaintree.Transaction{
		{
			Type: "SET_DATA",
			Payload: map[string]string{
				"path":  "down/in/the/thing",
				"value": "sometestvalue",
			},
		},
	})
	require.Nil(t, err)
	assert.Equal(t, resp.Tip.Bytes(), chain.Tip().Bytes())
	l.Debugw("first transaction done", "chainTip", chain.Tip().Bytes())

	t.Run("works on 2nd set", func(t *testing.T) {
		remoteTip := chain.Tip()
		height, _, _ := chain.ChainTree.Dag.Resolve([]string{"height"})
		l.Debugw("sending second transaction", "chainTip", chain.Tip().Bytes(), "height", height)

		resp, err := client.PlayTransactions(chain, treeKey, &remoteTip, []*chaintree.Transaction{
			{
				Type: "SET_DATA",
				Payload: map[string]string{
					"path":  "down/in/the/thing",
					"value": "sometestvalue",
				},
			},
		})
		require.Nil(t, err)
		assert.Equal(t, resp.Tip.Bytes(), chain.Tip().Bytes())
		l.Debugw("second transaction done", "respTip", resp.Tip.Bytes(), "chainTip", chain.Tip().Bytes())

		// and works a third time
		remoteTip = chain.Tip()
		resp, err = client.PlayTransactions(chain, treeKey, &remoteTip, []*chaintree.Transaction{
			{
				Type: "SET_DATA",
				Payload: map[string]string{
					"path":  "down/in/the/thing",
					"value": "sometestvalue",
				},
			},
		})
		require.Nil(t, err)
		assert.Equal(t, resp.Tip.Bytes(), chain.Tip().Bytes())
	})
}

func newTupeloSystem(ctx context.Context, testSet *testnotarygroup.TestSet) (*types.NotaryGroup, error) {
	ng := types.NewNotaryGroup("testnotary")
	for i, signKey := range testSet.SignKeys {
		sk := signKey
		signer := types.NewLocalSigner(testSet.PubKeys[i].ToEcdsaPub(), sk)
		syncer, err := actor.SpawnNamed(actors.NewTupeloNodeProps(&actors.TupeloConfig{
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
