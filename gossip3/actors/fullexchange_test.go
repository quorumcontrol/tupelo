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

	actorContext := actor.EmptyRootContext

	for i, signKey := range testSet.SignKeys {
		sk := signKey
		signer := types.NewLocalSigner(testSet.PubKeys[i].ToEcdsaPub(), sk)
		syncer, err := actorContext.SpawnNamed(NewTupeloNodeProps(&TupeloConfig{
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

	signer := signers[0]
	signer.Actor.PoisonFuture().Wait()

	newSyncer, err := actorContext.SpawnNamed(NewTupeloNodeProps(&TupeloConfig{
		Self:              signer,
		NotaryGroup:       notaryGroup,
		CurrentStateStore: storage.NewMemStorage(),
		PubSubSystem:      pubsub,
	}), "tupelo-"+signer.ID+"-2")
	require.Nil(t, err)
	signer.Actor = newSyncer

	// actor.EmptyRootContext.Send(signers[0].Actor, &actor.Restart{})
	// time.Sleep(3 * time.Second)

	time.Sleep(2000 * time.Millisecond)
	fmt.Println("Sending second round of transactions")

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

	fmt.Println("Completed second round of transactions")

	time.Sleep(3 * time.Second)

	// cli := client.New(notaryGroup, chains[0].MustId(), pubsub)
	// cli.Listen()
	// defer cli.Stop()
	// _, err := cli.PlayTransactions(chains[0], keys[0], tips[0], []*chaintree.Transaction{{
	// 	Type: consensus.TransactionTypeSetData,
	// 	Payload: &consensus.SetDataPayload{
	// 		Path:  "path/after-sync",
	// 		Value: "test",
	// 	},
	// }})
	// require.Nil(t, err)

	// for i := 0; i < numOfTrees; i++ {
	// 	key, err := crypto.GenerateKey()
	// 	keys[i] = key
	// 	require.Nil(t, err)
	// 	chain, err := consensus.NewSignedChainTree(key.PublicKey, nodestore.NewStorageBasedStore(storage.NewMemStorage()))
	// 	require.Nil(t, err)
	// 	chains[i] = chain

	// 	cli := client.New(notaryGroup, chain.MustId(), pubsub)
	// 	cli.Listen()

	// 	var remoteTip *cid.Cid

	// 	for i := 0; i < 3; i++ {
	// 		resp, err := cli.PlayTransactions(chain, key, remoteTip, []*chaintree.Transaction{{
	// 			Type: consensus.TransactionTypeSetData,
	// 			Payload: &consensus.SetDataPayload{
	// 				Path:  fmt.Sprintf("path/key-%d", i),
	// 				Value: "test",
	// 			},
	// 		}})
	// 		require.Nil(t, err)
	// 		remoteTip = resp.Tip
	// 	}

	// 	cli.Stop()
	// }

	// storeA.ForEach([]byte{}, func(key, value []byte) error {
	// 	count++
	// 	targetStoreValue, err := storeB.Get(key)
	// 	require.Nil(t, err)
	// 	require.Equal(t, value, targetStoreValue)
	// 	return nil
	// })

	// s0 := notaryGroup.AllSigners()[0]
	// s0.Actor.Stop()
	// s0.Actor.Start()

	// if err != nil {
	// 	return nil, err
	// }

	// unsignedBlock := &chaintree.BlockWithHeaders{
	// 	Block: chaintree.Block{
	// 		PreviousTip: nil,
	// 		Transactions: []*chaintree.Transaction{
	// 			{
	// 				Type: "SET_DATA",
	// 				Payload: map[string]string{
	// 					"path":  "down/in/the/thing",
	// 					"value": "hi",
	// 				},
	// 			},
	// 		},
	// 	},
	// }

	// nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	// emptyTree := consensus.NewEmptyTree(treeDID, nodeStore)
	// emptyTip := emptyTree.Tip
	// testTree, err := chaintree.NewChainTree(emptyTree, nil, consensus.DefaultTransactors)

	// chain, err := rpcs.GetChain(chainId)
	// if err != nil {
	// 	return nil, err
	// }

	// key, err := rpcs.getKey(keyAddr)
	// if err != nil {
	// 	return nil, err
	// }

	// var remoteTip cid.Cid
	// if !chain.IsGenesis() {
	// 	remoteTip = chain.Tip()
	// }

	// resp, err := cli.PlayTransactions(chain, key, &remoteTip, transactions)
	// if err != nil {
	// 	return nil, err
	// }

	// s0 := system.AllSigners()[0]

	// for i := 0; i < 10; i++ {
	// 	trans := testhelpers.NewValidTransaction(t)
	// 	cli := client.New(system, string(trans.ObjectID), pubsub)
	// 	err := cli.SendTransaction(&trans)
	// 	require.Nil(t, err)
	// }

	// s0.Actor.Stop()

	// rootContext := actor.EmptyRootContext
	// storeA := storage.NewMemStorage()
	// storageA := rootContext.Spawn(NewStorageProps(storeA))

	// fullExchangeConfig := &FullExchangeConfig{
	// 	ConflictSetRouter: router,
	// 	CurrentStateStore: tn.cfg.CurrentStateStore,
	// }
	// fullExchangeActor, err := context.SpawnNamed(NewFullExchangeProps(fullExchangeConfig), "fullExchange")
	// if err != nil {
	// 	panic(fmt.Sprintf("error spawning: %v", err))
	// }
	// tn.fullExchangeActor = fullExchangeActor

	// initialExchange := &messages.RequestFullExchange{
	// 	Destination: extmsgs.ToActorPid(tn.cfg.NotaryGroup.GetRandomSyncer()),
	// }
	// context.Send(tn.fullExchangeActor, initialExchange)

	// defer storageA.Poison()
	// storeB := storage.NewMemStorage()
	// storageB := rootContext.Spawn(NewStorageProps(storeB))
	// defer storageB.Poison()

	// for i := 1; i <= 5; i++ {
	// 	val := crypto.Keccak256([]byte{byte(i)})
	// 	key := crypto.Keccak256(val)
	// 	rootContext.Request(storageA, &extmsgs.Store{
	// 		Key:        key,
	// 		Value:      val,
	// 		SkipNotify: true,
	// 	})
	// }

	// for i := 6; i <= 10; i++ {
	// 	val := crypto.Keccak256([]byte{byte(i)})
	// 	key := crypto.Keccak256(val)
	// 	rootContext.Request(storageB, &extmsgs.Store{
	// 		Key:        key,
	// 		Value:      val,
	// 		SkipNotify: true,
	// 	})
	// }

	// exchangeA := rootContext.Spawn(NewFullExchangeProps(storageA))
	// defer exchangeA.Poison()
	// exchangeB := rootContext.Spawn(NewFullExchangeProps(storageB))
	// defer exchangeB.Poison()

	// rootContext.RequestFuture(exchangeA, &messages.RequestFullExchange{
	// 	DestinationHolder: messages.DestinationHolder{
	// 		Destination: extmsgs.ToActorPid(exchangeB),
	// 	},
	// }, 2*time.Second).Wait()

	// // Give time for Store messages to go through
	// time.Sleep(100 * time.Millisecond)

	// count := 0

	// storeA.ForEach([]byte{}, func(key, value []byte) error {
	// 	count++
	// 	targetStoreValue, err := storeB.Get(key)
	// 	require.Nil(t, err)
	// 	require.Equal(t, value, targetStoreValue)
	// 	return nil
	// })

	// require.Equal(t, count, 10)
}
