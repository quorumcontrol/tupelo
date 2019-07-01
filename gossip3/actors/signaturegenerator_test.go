package actors

import (
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	datastore "github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	"github.com/quorumcontrol/messages/build/go/services"
	"github.com/quorumcontrol/tupelo-go-sdk/consensus"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/testhelpers"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignatureGenerator(t *testing.T) {
	ts := testnotarygroup.NewTestSet(t, 1)
	signer := types.NewLocalSigner(consensus.PublicKeyToEcdsaPub(&ts.PubKeys[0]), ts.SignKeys[0])
	ng := types.NewNotaryGroup("signatureGenerator")
	ng.AddSigner(signer)
	currentState := dsync.MutexWrap(datastore.NewMapDatastore())
	rootContext := actor.EmptyRootContext
	cfg := &TransactionValidatorConfig{
		CurrentStateStore: currentState,
		NotaryGroup:       ng,
	}
	validator := rootContext.Spawn(NewTransactionValidatorProps(cfg))
	defer rootContext.Poison(validator)

	sigGenerator := rootContext.Spawn(NewSignatureGeneratorProps(signer, ng))
	defer rootContext.Poison(sigGenerator)

	fut := actor.NewFuture(5 * time.Second)
	validatorSenderFunc := func(context actor.Context) {
		switch msg := context.Message().(type) {
		case *services.AddBlockRequest:
			context.Request(validator, &validationRequest{
				transaction: msg,
			})
		case *messages.SignatureWrapper:
			context.Send(fut.PID(), msg)
		case *messages.TransactionWrapper:
			context.Request(sigGenerator, msg)
		}
	}

	sender := rootContext.Spawn(actor.PropsFromFunc(validatorSenderFunc))
	defer rootContext.Poison(sender)

	trans := testhelpers.NewValidTransaction(t)

	rootContext.Send(sender, &trans)

	msg, err := fut.Result()
	require.Nil(t, err)

	sigWrapper := msg.(*messages.SignatureWrapper)
	assert.Equal(t, uint32(1), sigWrapper.Signature.Signers[0])
}

func BenchmarkSignatureGenerator(b *testing.B) {
	ts := testnotarygroup.NewTestSet(b, 1)
	signer := types.NewLocalSigner(consensus.PublicKeyToEcdsaPub(&ts.PubKeys[0]), ts.SignKeys[0])
	ng := types.NewNotaryGroup("signatureGenerator")
	ng.AddSigner(signer)
	currentState := dsync.MutexWrap(datastore.NewMapDatastore())
	rootContext := actor.EmptyRootContext
	cfg := &TransactionValidatorConfig{
		CurrentStateStore: currentState,
		NotaryGroup:       ng,
	}
	validator := rootContext.Spawn(NewTransactionValidatorProps(cfg))
	defer rootContext.Poison(validator)

	sigGenerator := rootContext.Spawn(NewSignatureGeneratorProps(signer, ng))
	defer rootContext.Poison(sigGenerator)

	trans := testhelpers.NewValidTransaction(b)

	transWrapper, err := rootContext.RequestFuture(validator, &validationRequest{
		transaction: &trans,
	}, 1*time.Second).Result()
	require.Nil(b, err)

	second := 1 * time.Second
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := rootContext.RequestFuture(sigGenerator, transWrapper, second).Result()
		require.Nil(b, err)
	}
}
