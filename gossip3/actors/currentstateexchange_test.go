package actors

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/quorumcontrol/messages/build/go/transactions"
	"github.com/quorumcontrol/tupelo-go-sdk/consensus"

	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo/storage"

	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/tupelo-go-sdk/client"

	"github.com/AsynkronIT/protoactor-go/actor"
	cid "github.com/ipfs/go-cid"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/require"
)

func TestCurrentStateExchange(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	notaryGroupSize := 2
	testSet := testnotarygroup.NewTestSet(t, notaryGroupSize)
	pubsub := remote.NewSimulatedPubSub()
	signers := make([]*types.Signer, notaryGroupSize)

	rootContext := actor.EmptyRootContext

	notaryGroup := types.NewNotaryGroup("testnotary")
	defer func() {
		for _, signer := range notaryGroup.AllSigners() {
			rootContext.Poison(signer.Actor)
		}
	}()

	for i, signKey := range testSet.SignKeys {
		sk := signKey
		signer := types.NewLocalSigner(testSet.PubKeys[i], sk)
		syncer, err := rootContext.SpawnNamed(NewTupeloNodeProps(&TupeloConfig{
			Self:              signer,
			NotaryGroup:       notaryGroup,
			CurrentStateStore: storage.NewDefaultMemory(),
			PubSubSystem:      pubsub,
		}), "tupelo-"+signer.ID)
		require.Nil(t, err)
		signer.Actor = syncer
		signers[i] = signer
		notaryGroup.AddSigner(signer)
	}

	numOfTrees := 3
	keys := make([]*ecdsa.PrivateKey, numOfTrees)
	chains := make([]*consensus.SignedChainTree, numOfTrees)
	tips := make([]*cid.Cid, numOfTrees)

	// First sign some chaintrees on the notary group to fill up current state
	for i := 0; i < numOfTrees; i++ {
		key, err := crypto.GenerateKey()
		keys[i] = key
		require.Nil(t, err)
		chain, err := consensus.NewSignedChainTree(key.PublicKey, nodestore.MustMemoryStore(ctx))
		require.Nil(t, err)
		chains[i] = chain

		cli := client.New(notaryGroup, chain.MustId(), pubsub)
		cli.Listen()

		var remoteTip *cid.Cid

		for ti := 0; ti < 3; ti++ {
			txn, err := chaintree.NewSetDataTransaction(fmt.Sprintf("path/key-%d", ti), "test")
			require.Nil(t, err)

			resp, err := cli.PlayTransactions(chain, key, remoteTip, []*transactions.Transaction{txn})
			require.Nil(t, err)

			remoteTip = resp.Tip
		}

		tips[i] = remoteTip
		cli.Stop()
	}

	time.Sleep(100 * time.Millisecond)

	// Now stop signer0 and replace with new signer0 with empty CurrentStateStore
	signer := signers[0]
	err := rootContext.PoisonFuture(signer.Actor).Wait()
	require.Nil(t, err)

	newSyncer, err := rootContext.SpawnNamed(NewTupeloNodeProps(&TupeloConfig{
		Self:              signer,
		NotaryGroup:       notaryGroup,
		CurrentStateStore: storage.NewDefaultMemory(),
		PubSubSystem:      pubsub,
	}), "tupelo-"+signer.ID+"-2")
	require.Nil(t, err)
	signer.Actor = newSyncer

	time.Sleep(100 * time.Millisecond)

	// Check that notary group can sign transactions on top of existing state
	// Given the notary group size is 2, this proves that signer0 has pulled in CurrentState
	for i := 0; i < numOfTrees; i++ {
		key := keys[i]
		chain := chains[i]
		remoteTip := tips[i]

		cli := client.New(notaryGroup, chain.MustId(), pubsub)
		cli.Listen()

		txn, err := chaintree.NewSetDataTransaction("path/after-sync", "test")
		require.Nil(t, err)

		_, err = cli.PlayTransactions(chain, key, remoteTip, []*transactions.Transaction{txn})
		require.Nil(t, err)
		cli.Stop()
	}
}
