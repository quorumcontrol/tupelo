package actors

import (
	"strconv"
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/storage"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/testhelpers"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/types"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConflictSetRouterQuorum(t *testing.T) {
	numSigners := 3
	ts := testnotarygroup.NewTestSet(t, numSigners)
	ng, err := newActorlessSystem(ts)
	require.Nil(t, err)

	sigGeneratorActors := make([]*actor.PID, numSigners)
	for i, signer := range ng.AllSigners() {
		sg, err := actor.SpawnNamed(NewSignatureGeneratorProps(signer, ng), "sigGenerator-"+signer.ID)
		require.Nil(t, err)
		sigGeneratorActors[i] = sg
		defer sg.Poison()
	}
	signer := ng.AllSigners()[0]

	alwaysChecker := actor.Spawn(NewAlwaysVerifierProps())
	defer alwaysChecker.Poison()

	sender := actor.Spawn(NewNullActorProps())
	defer sender.Poison()

	cfg := &ConflictSetRouterConfig{
		NotaryGroup:        ng,
		Signer:             signer,
		SignatureChecker:   alwaysChecker,
		SignatureSender:    sender,
		SignatureGenerator: sigGeneratorActors[0],
		CurrentStateStore:  storage.NewMemStorage(),
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

	trans := testhelpers.NewValidTransaction(t)
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

	// test that it cleans up the actor:
	_, ok := actor.ProcessRegistry.GetLocal(conflictSetRouter.GetId() + "/" + string(conflictSetIDToInternalID([]byte(transWrapper.ConflictSetID))))
	assert.False(t, ok)

	// test that after done it won't create a new actor
	conflictSetRouter.Tell(transWrapper)
	time.Sleep(50 * time.Millisecond)

	_, ok = actor.ProcessRegistry.GetLocal(conflictSetRouter.GetId() + "/" + string(conflictSetIDToInternalID([]byte(transWrapper.ConflictSetID))))
	assert.False(t, ok)
}

func TestHandlesDeadlocks(t *testing.T) {
	numSigners := 3
	ts := testnotarygroup.NewTestSet(t, numSigners)
	ng, err := newActorlessSystem(ts)
	require.Nil(t, err)

	sigGeneratorActors := make([]*actor.PID, numSigners)
	for i, signer := range ng.AllSigners() {
		sg, err := actor.SpawnNamed(NewSignatureGeneratorProps(signer, ng), "sigGenerator-"+signer.ID)
		require.Nil(t, err)
		sigGeneratorActors[i] = sg
		defer sg.Poison()
	}
	signer := ng.AllSigners()[0]

	alwaysChecker := actor.Spawn(NewAlwaysVerifierProps())
	defer alwaysChecker.Poison()

	sender := actor.Spawn(NewNullActorProps())
	defer sender.Poison()

	cfg := &ConflictSetRouterConfig{
		NotaryGroup:        ng,
		Signer:             signer,
		SignatureChecker:   alwaysChecker,
		SignatureSender:    sender,
		SignatureGenerator: sigGeneratorActors[0],
		CurrentStateStore:  storage.NewMemStorage(),
	}

	fut := actor.NewFuture(10 * time.Second)

	isReadyFuture := actor.NewFuture(5 * time.Second)
	parentFunc := func(context actor.Context) {
		switch msg := context.Message().(type) {
		case *actor.Started:
			cs, err := context.SpawnNamed(NewConflictSetRouterProps(cfg), "testCSR")
			require.Nil(t, err)
			isReadyFuture.PID().Tell(cs)
		case *messages.CurrentStateWrapper:
			fut.PID().Tell(msg)
		}
	}

	parent, err := actor.SpawnNamed(actor.FromFunc(parentFunc), "THDParent")
	require.Nil(t, err)
	defer parent.Poison()

	keyBytes, err := hexutil.Decode("0xf9c0b741e7c065ea4fe4fde335c4ee575141db93236e3d86bb1c9ae6ccddf6f1")
	require.Nil(t, err)
	treeKey, err := crypto.ToECDSA(keyBytes)
	require.Nil(t, err)

	csInterface, err := isReadyFuture.Result()
	require.Nil(t, err)
	conflictSetRouter := csInterface.(*actor.PID)

	trans := make([]*messages.TransactionWrapper, len(sigGeneratorActors))
	var conflictSetID string
	for i := 0; i < len(trans); i++ {
		tr := testhelpers.NewValidTransactionWithPathAndValue(t, treeKey, "path/to/somewhere", strconv.Itoa(i))
		require.Truef(t, conflictSetID == "" || tr.ConflictSetID() == conflictSetID, "test transactions should all be in the same conflict set")
		conflictSetID = tr.ConflictSetID()
		trans[i] = fakeValidateTransaction(t, &tr)
		conflictSetRouter.Tell(trans[i])
	}
	// it's known that trans[0] is the lowest transaction,
	// this is just a sanity check
	require.True(t, string(trans[1].TransactionID) < string(trans[2].TransactionID))
	require.True(t, string(trans[1].TransactionID) < string(trans[0].TransactionID))

	// note skipping first signer here
	for i := 1; i < len(sigGeneratorActors); i++ {
		sig, err := sigGeneratorActors[i].RequestFuture(trans[i], 1*time.Second).Result()
		require.Nil(t, err)
		conflictSetRouter.Tell(sig)
	}

	// at this point the first signer should have 3 transactions with 1 signature each and be in a deadlocked state
	// which means it should sign the lowest transaction (a different one than it did before)
	// one more signature on that same transaction should get it to quorum in the new view
	sig, err := sigGeneratorActors[1].RequestFuture(trans[1], 1*time.Second).Result()
	require.Nil(t, err)
	conflictSetRouter.Tell(sig)

	msg, err := fut.Result()
	require.Nil(t, err)
	wrap := msg.(*messages.CurrentStateWrapper)
	assert.True(t, wrap.Verified)
	assert.Equal(t, trans[1].Transaction.NewTip, wrap.CurrentState.Signature.NewTip)
}

func TestHandlesCommitsBeforeTransactions(t *testing.T) {
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

	sender := actor.Spawn(testhelpers.NewNullActorProps())
	defer sender.Poison()

	cfg := &ConflictSetRouterConfig{
		NotaryGroup:        ng,
		Signer:             signer,
		SignatureChecker:   alwaysChecker,
		SignatureSender:    sender,
		SignatureGenerator: sigGeneratorActors[0],
		CurrentStateStore:  storage.NewMemStorage(),
	}

	fut0 := actor.NewFuture(10 * time.Second)

	isReadyFuture0 := actor.NewFuture(5 * time.Second)
	parentFunc0 := func(context actor.Context) {
		switch msg := context.Message().(type) {
		case *actor.Started:
			cs, err := context.SpawnNamed(NewConflictSetRouterProps(cfg), "testCSR0")
			require.Nil(t, err)
			isReadyFuture0.PID().Tell(cs)
		case *messages.CurrentStateWrapper:
			fut0.PID().Tell(msg)
		}
	}

	parent0, err := actor.SpawnNamed(actor.FromFunc(parentFunc0), "THCBTParent0")
	require.Nil(t, err)
	defer parent0.Poison()

	csInterface0, err := isReadyFuture0.Result()
	require.Nil(t, err)
	conflictSetRouter0 := csInterface0.(*actor.PID)

	trans := testhelpers.NewValidTransaction(t)
	transWrapper := fakeValidateTransaction(t, &trans)

	conflictSetRouter0.Tell(transWrapper)

	// note skipping first signer here
	for i := 1; i < len(sigGeneratorActors); i++ {
		sig, err := sigGeneratorActors[i].RequestFuture(transWrapper, 1*time.Second).Result()
		require.Nil(t, err)
		conflictSetRouter0.Tell(sig)
	}

	msg, err := fut0.Result()
	require.Nil(t, err)

	currentStateWrapper := msg.(*messages.CurrentStateWrapper)

	// setup another CSR to send the commit to first
	fut1 := actor.NewFuture(10 * time.Second)

	isReadyFuture1 := actor.NewFuture(5 * time.Second)
	parentFunc1 := func(context actor.Context) {
		switch msg := context.Message().(type) {
		case *actor.Started:
			cs, err := context.SpawnNamed(NewConflictSetRouterProps(cfg), "testCSR1")
			require.Nil(t, err)
			isReadyFuture1.PID().Tell(cs)
		case *messages.CurrentStateWrapper:
			fut1.PID().Tell(msg)
		}
	}

	parent1, err := actor.SpawnNamed(actor.FromFunc(parentFunc1), "THCBTParent1")
	require.Nil(t, err)
	defer parent1.Poison()

	csInterface1, err := isReadyFuture1.Result()
	require.Nil(t, err)
	conflictSetRouter1 := csInterface1.(*actor.PID)

	conflictSetRouter1.Tell(&commitNotification{
		objectID:   trans.ObjectID,
		store:      &extmsgs.Store{
			Key:        currentStateWrapper.CurrentState.CommittedKey(),
			Value:      currentStateWrapper.Value,
			SkipNotify: currentStateWrapper.Internal,
		},
		height:     currentStateWrapper.CurrentState.Signature.Height,
		nextHeight: 0,
	})

	msg, err = fut1.Result()
	require.Nil(t, err)
	wrap := msg.(*messages.CurrentStateWrapper)
	assert.True(t, wrap.Verified)
	assert.Equal(t, trans.NewTip, wrap.CurrentState.Signature.NewTip)
}

func fakeValidateTransaction(t testing.TB, trans *extmsgs.Transaction) *messages.TransactionWrapper {
	bits, err := trans.MarshalMsg(nil)
	require.Nil(t, err)
	key := crypto.Keccak256(bits)

	wrapper := &messages.TransactionWrapper{
		TransactionID: key,
		Transaction:   trans,
		Key:           key,
		Value:         bits,
		ConflictSetID: trans.ConflictSetID(),
		PreFlight:     true,
		Accepted:      true,
		Metadata:      messages.MetadataMap{"seen": time.Now()},
	}
	return wrapper
}

func newActorlessSystem(testSet *testnotarygroup.TestSet) (*types.NotaryGroup, error) {
	ng := types.NewNotaryGroup("actorless")
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
	return actor.PropsFromProducer(func() actor.Actor {
		return new(AlwaysVerifier)
	}).WithReceiverMiddleware(
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

func NewNeverVerifierProps() *actor.Props {
	return actor.PropsFromProducer(func() actor.Actor {
		return new(NeverVerifier)
	}).WithReceiverMiddleware(
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

var nullActorFunc = func(_ actor.Context) {}

func NewNullActorProps() *actor.Props {
	return actor.PropsFromFunc(nullActorFunc)
}
