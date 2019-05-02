package actors

import (
	"fmt"
	"testing"
	"time"

	"github.com/quorumcontrol/tupelo-go-sdk/consensus"

	"crypto/ecdsa"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo-go-sdk/client"

	"github.com/AsynkronIT/protoactor-go/actor"
	cid "github.com/ipfs/go-cid"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/require"
)

func TestFullExchange(t *testing.T) {
	notaryGroupSize := 2
	testSet := testnotarygroup.NewTestSet(t, notaryGroupSize)
	pubsub := remote.NewSimulatedPubSub()
	signers := make([]*types.Signer, notaryGroupSize)

	notaryGroup := types.NewNotaryGroup("testnotary")
	defer func() {
		for _, signer := range notaryGroup.AllSigners() {
			signer.Actor.Poison()
		}
	}()

	for i, signKey := range testSet.SignKeys {
		sk := signKey
		signer := types.NewLocalSigner(testSet.PubKeys[i].ToEcdsaPub(), sk)
		syncer, err := actor.EmptyRootContext.SpawnNamed(NewTupeloNodeProps(&TupeloConfig{
			Self:              signer,
			NotaryGroup:       notaryGroup,
			CurrentStateStore: storage.NewMemStorage(),
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

	for i := 0; i < numOfTrees; i++ {
		key, err := crypto.GenerateKey()
		keys[i] = key
		require.Nil(t, err)
		chain, err := consensus.NewSignedChainTree(key.PublicKey, nodestore.NewStorageBasedStore(storage.NewMemStorage()))
		require.Nil(t, err)
		chains[i] = chain

		cli := client.New(notaryGroup, chain.MustId(), pubsub)
		cli.Listen()

		var remoteTip *cid.Cid

		for ti := 0; ti < 3; ti++ {
			resp, err := cli.PlayTransactions(chain, key, remoteTip, []*chaintree.Transaction{{
				Type: consensus.TransactionTypeSetData,
				Payload: &consensus.SetDataPayload{
					Path:  fmt.Sprintf("path/key-%d", ti),
					Value: "test",
				},
			}})
			require.Nil(t, err)
			remoteTip = resp.Tip
		}

		tips[i] = remoteTip
		cli.Stop()
	}

	time.Sleep(100 * time.Millisecond)

	signer := signers[0]
	err := signer.Actor.PoisonFuture().Wait()
	require.Nil(t, err)

	newSyncer, err := actor.EmptyRootContext.SpawnNamed(NewTupeloNodeProps(&TupeloConfig{
		Self:              signer,
		NotaryGroup:       notaryGroup,
		CurrentStateStore: storage.NewMemStorage(),
		PubSubSystem:      pubsub,
	}), "tupelo-"+signer.ID+"-2")
	require.Nil(t, err)
	signer.Actor = newSyncer

	time.Sleep(100 * time.Millisecond)

	for i := 0; i < numOfTrees; i++ {
		key := keys[i]
		chain := chains[i]
		remoteTip := tips[i]

		cli := client.New(notaryGroup, chain.MustId(), pubsub)
		cli.Listen()

		_, err := cli.PlayTransactions(chain, key, remoteTip, []*chaintree.Transaction{{
			Type: consensus.TransactionTypeSetData,
			Payload: &consensus.SetDataPayload{
				Path:  "path/after-sync",
				Value: "test",
			},
		}})
		require.Nil(t, err)
		cli.Stop()
	}
}