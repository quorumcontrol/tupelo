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
	rootContext := actor.EmptyRootContext
	validator := rootContext.Spawn(NewTransactionValidatorProps(currentState))
	defer validator.Poison()

	fut := actor.NewFuture(1 * time.Second)
	validatorSenderFunc := func(context actor.Context) {
		switch msg := context.Message().(type) {
		case *extmsgs.Transaction:
			context.Request(validator, &validationRequest{
				transaction: msg,
			})
		case *messages.TransactionWrapper:
			context.Send(fut.PID(), msg)
		}
	}

	sender := rootContext.Spawn(actor.PropsFromFunc(validatorSenderFunc))
	defer sender.Poison()

	trans := testhelpers.NewValidTransaction(t)

	rootContext.Send(sender, &trans)

	_, err := fut.Result()
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

	tokenName := "evilTuples"

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: nil,
			Height:      0,
			Transactions: []*chaintree.Transaction{
				{
					Type: "MINT_TOKEN",
					Payload: &consensus.MintTokenPayload{
						Name:   tokenName,
						Amount: 4999,
					},
				},
			},
		},
	}

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	evilTree := consensus.NewEmptyTree(treeDID, nodeStore)

	path, err := consensus.DecodePath("tree/" + consensus.TreePathForTokens)
	require.Nil(t, err)

	tokenPath := append(path, tokenName)
	monetaryPolicyPath := append(tokenPath, "monetaryPolicy")

	evilTree, err = evilTree.SetAsLink(monetaryPolicyPath, &consensus.TokenMonetaryPolicy{
		Maximum: 5000,
	})
	require.Nil(t, err)

	testTree, err := chaintree.NewChainTree(evilTree, nil, consensus.DefaultTransactors)
	require.Nil(t, err)

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	require.Nil(t, err)

	valid, err := testTree.ProcessBlock(blockWithHeaders)
	require.Equal(t, true, valid)
	require.Nil(t, err)
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

	fut := actor.EmptyRootContext.RequestFuture(validator, &validationRequest{
		transaction: &trans,
	}, 5*time.Second)
	isValid, err := fut.Result()
	require.False(t, isValid.(*messages.TransactionWrapper).Accepted)

	require.Nil(t, err)
}

func BenchmarkValidator(b *testing.B) {
	currentState := storage.NewMemStorage()
	rootContext := actor.EmptyRootContext
	validator := rootContext.Spawn(NewTransactionValidatorProps(currentState))
	defer validator.Poison()

	trans := testhelpers.NewValidTransaction(b)

	futures := make([]*actor.Future, b.N)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		f := rootContext.RequestFuture(validator, &validationRequest{
			transaction: &trans,
		}, 5*time.Second)
		futures[i] = f
	}
	for _, f := range futures {
		_, err := f.Result()
		require.Nil(b, err)
	}
}
