package actors

import (
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/Workiva/go-datastructures/bitarray"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/testhelpers"
	"github.com/quorumcontrol/tupelo/gossip3/types"
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

	sigGenerator := actor.Spawn(NewSignatureGeneratorProps(signer, ng))
	defer sigGenerator.Poison()

	sigSender := actor.Spawn(NewSignatureSenderProps())
	defer sigSender.Poison()

	sigChecker := actor.Spawn(NewSignatureVerifier())
	defer sigChecker.Poison()

	conflictSetCfg := ConflictSetConfig{
		ID: "test-sig-gen",
		NotaryGroup: ng,
		Signer: signer,
		SignatureGenerator: sigGenerator,
		SignatureSender: sigSender,
		SignatureChecker: sigChecker,
		CurrentStateStore: currentState,
	}
	conflictSet := actor.Spawn(NewConflictSetProps(&conflictSetCfg))
	defer conflictSet.Poison()

	fut := actor.NewFuture(5 * time.Second)
	validatorSenderFunc := func(context actor.Context) {
		switch msg := context.Message().(type) {
		case *messages.Store:
			context.Request(conflictSet, msg)
		case *messages.SignatureWrapper:
			fut.PID().Tell(msg)
		case *messages.TransactionWrapper:
			context.Request(sigGenerator, msg)
		}
	}

	sender := actor.Spawn(actor.FromFunc(validatorSenderFunc))
	defer sender.Poison()

	trans := testhelpers.NewValidTransaction(t)
	value, err := trans.MarshalMsg(nil)
	require.Nil(t, err)
	key := crypto.Keccak256(value)

	sender.Tell(&messages.Store{
		Key:   key,
		Value: value,
	})

	msg, err := fut.Result()
	require.Nil(t, err)

	sigWrapper := msg.(*messages.SignatureWrapper)
	arry, err := bitarray.Unmarshal(sigWrapper.Signature.Signers)
	require.Nil(t, err)
	isSet, err := arry.GetBit(0)
	require.Nil(t, err)
	assert.True(t, isSet)
}

func BenchmarkSignatureGenerator(b *testing.B) {
	ts := testnotarygroup.NewTestSet(b, 1)
	signer := types.NewLocalSigner(ts.PubKeys[0].ToEcdsaPub(), ts.SignKeys[0])
	ng := types.NewNotaryGroup("signatureGenerator")
	ng.AddSigner(signer)
	currentState := storage.NewMemStorage()

	sigGenerator := actor.Spawn(NewSignatureGeneratorProps(signer, ng))
	defer sigGenerator.Poison()

	sigSender := actor.Spawn(NewSignatureSenderProps())
	defer sigSender.Poison()

	sigChecker := actor.Spawn(NewSignatureVerifier())
	defer sigChecker.Poison()

	conflictSetCfg := ConflictSetConfig{
		ID: "test-sig-gen",
		NotaryGroup: ng,
		Signer: signer,
		SignatureGenerator: sigGenerator,
		SignatureSender: sigSender,
		SignatureChecker: sigChecker,
		CurrentStateStore: currentState,
	}
	conflictSet := actor.Spawn(NewConflictSetProps(&conflictSetCfg))
	defer conflictSet.Poison()

	trans := testhelpers.NewValidTransaction(b)
	value, err := trans.MarshalMsg(nil)
	require.Nil(b, err)
	key := crypto.Keccak256(value)

	transWrapper, err := conflictSet.RequestFuture(&messages.Store{
		Key:   key,
		Value: value,
	}, 1*time.Second).Result()
	require.Nil(b, err)

	second := 1 * time.Second
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := sigGenerator.RequestFuture(transWrapper, second).Result()
		require.Nil(b, err)
	}
}
