package actors

import (
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/Workiva/go-datastructures/bitarray"
	"github.com/quorumcontrol/storage"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/testhelpers"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/types"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSignatureGenerator(t *testing.T) {
	ts := testnotarygroup.NewTestSet(t, 1)
	signer := types.NewLocalSigner(ts.PubKeys[0].ToEcdsaPub(), ts.SignKeys[0])
	ng := types.NewNotaryGroup("signatureGenerator")
	ng.AddSigner(signer)
	currentState := storage.NewMemStorage()
	rootContext := actor.EmptyRootContext
	validator := rootContext.Spawn(NewTransactionValidatorProps(currentState))
	defer validator.Poison()

	sigGenerator := rootContext.Spawn(NewSignatureGeneratorProps(signer, ng))
	defer sigGenerator.Poison()

	fut := actor.NewFuture(5 * time.Second)
	validatorSenderFunc := func(context actor.Context) {
		switch msg := context.Message().(type) {
		case *extmsgs.Transaction:
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
	defer sender.Poison()

	trans := testhelpers.NewValidTransaction(t)

	rootContext.Send(sender, &trans)

	msg, err := fut.Result()
	require.Nil(t, err)

	sigWrapper := msg.(*messages.SignatureWrapper)
	array, err := bitarray.Unmarshal(sigWrapper.Signature.Signers)
	require.Nil(t, err)
	isSet, err := array.GetBit(0)
	require.Nil(t, err)
	assert.True(t, isSet)
}

func BenchmarkSignatureGenerator(b *testing.B) {
	ts := testnotarygroup.NewTestSet(b, 1)
	signer := types.NewLocalSigner(ts.PubKeys[0].ToEcdsaPub(), ts.SignKeys[0])
	ng := types.NewNotaryGroup("signatureGenerator")
	ng.AddSigner(signer)
	currentState := storage.NewMemStorage()
	rootContext := actor.EmptyRootContext
	validator := rootContext.Spawn(NewTransactionValidatorProps(currentState))
	defer validator.Poison()

	sigGnerator := rootContext.Spawn(NewSignatureGeneratorProps(signer, ng))
	defer sigGnerator.Poison()

	trans := testhelpers.NewValidTransaction(b)

	transWrapper, err := rootContext.RequestFuture(validator, &validationRequest{
		transaction: &trans,
	}, 1*time.Second).Result()
	require.Nil(b, err)

	second := 1 * time.Second
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := rootContext.RequestFuture(sigGnerator, transWrapper, second).Result()
		require.Nil(b, err)
	}
}
