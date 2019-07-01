package actors

import (
	"context"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	datastore "github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/build/go/services"
	"github.com/quorumcontrol/messages/build/go/transactions"
	"github.com/quorumcontrol/tupelo-go-sdk/consensus"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/testhelpers"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/stretchr/testify/require"

	"github.com/quorumcontrol/tupelo/gossip3/messages"
)

func TestValidator(t *testing.T) {
	currentState := dsync.MutexWrap(datastore.NewMapDatastore())
	rootContext := actor.EmptyRootContext
	cfg := &TransactionValidatorConfig{
		CurrentStateStore: currentState,
		NotaryGroup:       types.NewNotaryGroup("testvalidator"),
	}
	validator := rootContext.Spawn(NewTransactionValidatorProps(cfg))
	defer rootContext.Poison(validator)

	fut := actor.NewFuture(1 * time.Second)
	validatorSenderFunc := func(context actor.Context) {
		switch msg := context.Message().(type) {
		case *services.AddBlockRequest:
			context.Request(validator, &validationRequest{
				transaction: msg,
			})
		case *messages.TransactionWrapper:
			context.Send(fut.PID(), msg)
		}
	}

	sender := rootContext.Spawn(actor.PropsFromFunc(validatorSenderFunc))
	defer rootContext.Poison(sender)

	trans := testhelpers.NewValidTransaction(t)

	rootContext.Send(sender, &trans)

	_, err := fut.Result()
	require.Nil(t, err)
}

func TestCannotFakeOldHistory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// this test makes sure that you can't send in old history on the
	// genesis transaction and get the notary group to approve your first
	// transaction.

	currentState := dsync.MutexWrap(datastore.NewMapDatastore())
	cfg := &TransactionValidatorConfig{
		CurrentStateStore: currentState,
		NotaryGroup:       types.NewNotaryGroup("testcannotfakeoldhistory"),
	}
	validator := actor.EmptyRootContext.Spawn(NewTransactionValidatorProps(cfg))
	defer actor.EmptyRootContext.Poison(validator)

	treeKey, err := crypto.GenerateKey()
	require.Nil(t, err)

	sw := safewrap.SafeWrap{}

	treeDID := consensus.AddrToDid(crypto.PubkeyToAddress(treeKey.PublicKey).String())

	tokenName := "evilTuples"
	tokenFullName := consensus.TokenName{ChainTreeDID: treeDID, LocalName: tokenName}

	mintTxn, err := chaintree.NewMintTokenTransaction(tokenName, 4999)
	require.Nil(t, err)

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       0,
			Transactions: []*transactions.Transaction{mintTxn},
		},
	}

	nodeStore := nodestore.MustMemoryStore(ctx)
	evilTree := consensus.NewEmptyTree(treeDID, nodeStore)

	path, err := consensus.DecodePath("tree/" + consensus.TreePathForTokens)
	require.Nil(t, err)

	tokenPath := append(path, tokenFullName.String())
	monetaryPolicyPath := append(tokenPath, "monetaryPolicy")

	evilTree, err = evilTree.SetAsLink(ctx, monetaryPolicyPath, &transactions.TokenMonetaryPolicy{
		Maximum: 5000,
	})
	require.Nil(t, err)

	balancePath := append(tokenPath, "balance")

	evilTree, err = evilTree.Set(ctx, balancePath, uint64(0))
	require.Nil(t, err)

	testTree, err := chaintree.NewChainTree(ctx, evilTree, nil, consensus.DefaultTransactors)
	require.Nil(t, err)

	blockWithHeaders, err := consensus.SignBlock(unsignedBlock, treeKey)
	require.Nil(t, err)

	valid, err := testTree.ProcessBlock(ctx, blockWithHeaders)
	require.Nil(t, err)
	require.True(t, valid)
	nodes := dagToByteNodes(t, evilTree)

	bits := sw.WrapObject(blockWithHeaders).RawData()
	require.Nil(t, sw.Err)

	trans := services.AddBlockRequest{
		PreviousTip: evilTree.Tip.Bytes(),
		Height:      blockWithHeaders.Height,
		NewTip:      testTree.Dag.Tip.Bytes(),
		Payload:     bits,
		State:       nodes,
		ObjectId:    []byte(treeDID),
	}

	fut := actor.EmptyRootContext.RequestFuture(validator, &validationRequest{
		transaction: &trans,
	}, 5*time.Second)
	isValid, err := fut.Result()
	require.False(t, isValid.(*messages.TransactionWrapper).Accepted)

	require.Nil(t, err)
}

func BenchmarkValidator(b *testing.B) {
	currentState := dsync.MutexWrap(datastore.NewMapDatastore())
	rootContext := actor.EmptyRootContext
	cfg := &TransactionValidatorConfig{
		CurrentStateStore: currentState,
		NotaryGroup:       types.NewNotaryGroup("BenchmarkValidator"),
	}
	validator := rootContext.Spawn(NewTransactionValidatorProps(cfg))
	defer actor.EmptyRootContext.Poison(validator)

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
