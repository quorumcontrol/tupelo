package actors

import (
	"strconv"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/types"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConflictSetRouterQuorum(t *testing.T) {
	numSigners := 3
	ts := testnotarygroup.NewTestSet(t, numSigners)
	ng, err := newActorlessSystem(ts)
	require.Nil(t, err)

	sigGeneratorActors := make([]*actor.PID, numSigners, numSigners)
	for i, signer := range ng.AllSigners() {
		sg, err := actor.SpawnNamed(NewSignatureGeneratorProps(signer, ng), "sigGenerator-"+signer.ID)
		require.Nil(t, err)
		sigGeneratorActors[i] = sg
		defer sg.Poison()
	}
	signer := ng.AllSigners()[0]

	alwaysChecker := actor.Spawn(NewAlwaysVerifierProps())
	defer alwaysChecker.Poison()

	sender := actor.Spawn(newNullActorProps())
	defer sender.Poison()

	cfg := &ConflictSetRouterConfig{
		NotaryGroup:        ng,
		Signer:             signer,
		SignatureChecker:   alwaysChecker,
		SignatureSender:    sender,
		SignatureGenerator: sigGeneratorActors[0],
	}

	var conflictSetRouter *actor.PID
	fut := actor.NewFuture(5 * time.Second)

	isReadyFuture := actor.NewFuture(5 * time.Second)
	parentFunc := func(context actor.Context) {
		switch msg := context.Message().(type) {
		case *actor.Started:
			cs, err := context.SpawnNamed(NewConflictSetRouterProps(cfg), "testCSR")
			require.Nil(t, err)
			conflictSetRouter = cs
			isReadyFuture.PID().Tell(true)
		case *messages.CurrentStateWrapper:
			fut.PID().Tell(msg)
		}
	}

	parent := actor.Spawn(actor.FromFunc(parentFunc))
	defer parent.Poison()

	t.Run("reaches quorum", func(t *testing.T) {
		trans := newValidTransaction(t)
		transWrapper := fakeValidateTransaction(t, &trans)

		_, err = isReadyFuture.Result()
		require.Nil(t, err)
		conflictSetRouter.Tell(transWrapper)

		// note skipping first signer here
		for i := 1; i < len(sigGeneratorActors); i++ {
			sig, err := sigGeneratorActors[i].RequestFuture(transWrapper, 1*time.Second).Result()
			require.Nil(t, err)
			conflictSetRouter.Tell(sig)
		}

		msg, err := fut.Result()
		require.Nil(t, err)
		assert.True(t, msg.(*messages.CurrentStateWrapper).Verified)
	})

	t.Run("handles deadlocks", func(t *testing.T) {
		keyBytes, err := hexutil.Decode("0xf9c0b741e7c065ea4fe4fde335c4ee575141db93236e3d86bb1c9ae6ccddf6f1")
		require.Nil(t, err)
		treeKey, err := crypto.ToECDSA(keyBytes)
		require.Nil(t, err)

		_, err = isReadyFuture.Result()
		require.Nil(t, err)

		trans := make([]*messages.TransactionWrapper, len(sigGeneratorActors))
		var conflictSetID string
		for i := 0; i < len(trans); i++ {
			tr := newValidTransactionWithPathAndValue(t, treeKey, "path/to/somewhere", strconv.Itoa(i))
			require.Truef(t, conflictSetID == "" || tr.ConflictSetID() == conflictSetID, "test transactions should all be in the same conflict set")
			conflictSetID = tr.ConflictSetID()
			trans[i] = fakeValidateTransaction(t, &tr)
			conflictSetRouter.Tell(trans[i])
		}
		// it's known that trans[2] is the lowest transaction,
		// this is just a sanity check
		require.True(t, string(trans[2].TransactionID) < string(trans[1].TransactionID))
		require.True(t, string(trans[2].TransactionID) < string(trans[0].TransactionID))

		// note skipping first signer here
		for i := 1; i < len(sigGeneratorActors); i++ {
			sig, err := sigGeneratorActors[i].RequestFuture(trans[i], 1*time.Second).Result()
			require.Nil(t, err)
			conflictSetRouter.Tell(sig)
		}

		// at this point the first signer should have 3 transactions with 1 signature each and be in a deadlocked state
		// which means it should sign the lowest transaction (a different one than it did before)
		// one more signature on that same transaction should get it to quorum in the new view
		// note skipping first signer here
		sig, err := sigGeneratorActors[1].RequestFuture(trans[2], 1*time.Second).Result()
		require.Nil(t, err)
		conflictSetRouter.Tell(sig)

		msg, err := fut.Result()
		require.Nil(t, err)
		assert.True(t, msg.(*messages.CurrentStateWrapper).Verified)
	})

}

func fakeValidateTransaction(t testing.TB, trans *messages.Transaction) *messages.TransactionWrapper {
	bits, err := trans.MarshalMsg(nil)
	require.Nil(t, err)
	key := crypto.Keccak256(bits)

	wrapper := &messages.TransactionWrapper{
		TransactionID: key,
		Transaction:   trans,
		Key:           key,
		Value:         bits,
		ConflictSetID: trans.ConflictSetID(),
		Accepted:      true,
		Metadata:      messages.MetadataMap{"seen": time.Now()},
	}
	return wrapper
}

func newActorlessSystem(testSet *testnotarygroup.TestSet) (*types.NotaryGroup, error) {
	ng := types.NewNotaryGroup()
	for i, signKey := range testSet.SignKeys {
		sk := signKey
		signer := types.NewLocalSigner(testSet.PubKeys[i].ToEcdsaPub(), sk)
		ng.AddSigner(signer)
	}
	return ng, nil
}

type AlwaysVerifier struct {
	middleware.LogAwareHolder
}

func NewAlwaysVerifierProps() *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return new(AlwaysVerifier)
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (av *AlwaysVerifier) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.SignatureVerification:
		msg.Verified = true
		context.Respond(msg)
	}
}

type NeverVerifier struct {
	middleware.LogAwareHolder
}

// TODO: this should have many workers, but a single
// point of entry so that it is easy to gang up signatures to verify
func NewNeverVerifierProps() *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return new(NeverVerifier)
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (nv *NeverVerifier) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.SignatureVerification:
		msg.Verified = false
		context.Respond(msg)
	}
}