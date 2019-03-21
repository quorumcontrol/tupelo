package actors

import (
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo-go-client/consensus"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/testhelpers"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/stretchr/testify/require"
)

func TestValidator(t *testing.T) {
	currentState := storage.NewMemStorage()
	validator := actor.Spawn(NewTransactionValidatorProps(currentState))
	defer validator.Poison()

	fut := actor.NewFuture(1 * time.Second)
	validatorSenderFunc := func(context actor.Context) {
		switch msg := context.Message().(type) {
		case *extmsgs.Store:
			context.Request(validator, &validationRequest{
				key:   msg.Key,
				value: msg.Value,
			})
		case *messages.TransactionWrapper:
			fut.PID().Tell(msg)
		}
	}

	sender := actor.Spawn(actor.FromFunc(validatorSenderFunc))
	defer sender.Poison()

	trans := testhelpers.NewValidTransaction(t)
	value, err := trans.MarshalMsg(nil)
	require.Nil(t, err)
	key := crypto.Keccak256(value)

	sender.Tell(&extmsgs.Store{
		Key:   key,
		Value: value,
	})

	_, err = fut.Result()
	require.Nil(t, err)
}

func TestCannotFakeOldHistory(t *testing.T) {
	// this test makes sure that you can't send in old history on the
	// genesis transaction and get the notary group to approve your first
	// transaction.

	currentState := storage.NewMemStorage()
	validator := actor.Spawn(NewTransactionValidatorProps(currentState))
	defer validator.Poison()

	treeKey, err := crypto.GenerateKey()
	require.Nil(t, err)

	sw := safewrap.SafeWrap{}

	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())

	coinName := "evilTuples"

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: nil,
			Height:      0,
			Transactions: []*chaintree.Transaction{
				{
					Type: "MINT_COIN",
					Payload: &consensus.MintCoinPayload{
						Name:   coinName,
						Amount: 4999,
					},
				},
			},
		},
	}

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	evilTree := consensus.NewEmptyTree(treeDID, nodeStore)

	path, err := consensus.DecodePath("tree/" + consensus.TreePathForCoins)
	require.Nil(t, err)

	coinPath := append(path, coinName)
	monetaryPolicyPath := append(coinPath, "monetaryPolicy")

	evilTree, err = evilTree.SetAsLink(monetaryPolicyPath, &consensus.CoinMonetaryPolicy{
		Maximum: 5000,
	})
	require.Nil(t, err)

	testTree, err := chaintree.NewChainTree(evilTree, nil, consensus.DefaultTransactors)
	require.Nil(t, err)

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	require.Nil(t, err)

	testTree.ProcessBlock(blockWithHeaders)
	nodes := dagToByteNodes(t, evilTree)

	bits := sw.WrapObject(blockWithHeaders).RawData()
	require.Nil(t, sw.Err)

	trans := extmsgs.Transaction{
		PreviousTip: evilTree.Tip.Bytes(),
		Height:      blockWithHeaders.Height,
		NewTip:      testTree.Dag.Tip.Bytes(),
		Payload:     bits,
		State:       nodes,
		ObjectID:    []byte(treeDID),
	}

	value, err := trans.MarshalMsg(nil)
	require.Nil(t, err)
	key := crypto.Keccak256(value)

	fut := validator.RequestFuture(&validationRequest{
		key:   key,
		value: value,
	}, 5*time.Second)
	isValid, err := fut.Result()
	require.False(t, isValid.(*messages.TransactionWrapper).Accepted)

	require.Nil(t, err)
}

func BenchmarkValidator(b *testing.B) {
	currentState := storage.NewMemStorage()
	validator := actor.Spawn(NewTransactionValidatorProps(currentState))
	defer validator.Poison()

	trans := testhelpers.NewValidTransaction(b)
	value, err := trans.MarshalMsg(nil)
	require.Nil(b, err)
	key := crypto.Keccak256(value)

	futures := make([]*actor.Future, b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f := validator.RequestFuture(&validationRequest{
			key:   key,
			value: value,
		}, 5*time.Second)
		futures[i] = f
	}
	for _, f := range futures {
		_, err := f.Result()
		require.Nil(b, err)
	}
}
