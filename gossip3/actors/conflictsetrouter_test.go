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

	rootContext := actor.EmptyRootContext

	sigGeneratorActors := make([]*actor.PID, numSigners)
	for i, signer := range ng.AllSigners() {
		sg, err := rootContext.SpawnNamed(NewSignatureGeneratorProps(signer, ng), "sigGenerator-"+signer.ID)
		require.Nil(t, err)
		sigGeneratorActors[i] = sg
		defer sg.Poison()
	}
	signer := ng.AllSigners()[0]

	alwaysChecker := rootContext.Spawn(NewAlwaysVerifierProps())
	defer alwaysChecker.Poison()

	sender := rootContext.Spawn(NewNullActorProps())
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
			context.Send(isReadyFuture.PID(), true)
		case *messages.CurrentStateWrapper:
			context.Send(fut.PID(), msg)
		}
	}

	parent := rootContext.Spawn(actor.PropsFromFunc(parentFunc))
	defer parent.Poison()

	trans := testhelpers.NewValidTransaction(t)
	transWrapper := fakeValidateTransaction(t, &trans)

	_, err = isReadyFuture.Result()
	require.Nil(t, err)
	rootContext.Send(conflictSetRouter, transWrapper)

	// note skipping first signer here
	for i := 1; i < len(sigGeneratorActors); i++ {
		sig, err := rootContext.RequestFuture(sigGeneratorActors[i], transWrapper, 1*time.Second).Result()
		require.Nil(t, err)
		rootContext.Send(conflictSetRouter, sig)
	}

	msg, err := fut.Result()
	require.Nil(t, err)
	assert.True(t, msg.(*messages.CurrentStateWrapper).Verified)

	// test that it cleans up the actor:
	_, ok := actor.ProcessRegistry.GetLocal(conflictSetRouter.GetId() + "/" + string(conflictSetIDToInternalID([]byte(transWrapper.ConflictSetID))))
	assert.False(t, ok)

	// test that after done it won't create a new actor
	rootContext.Send(conflictSetRouter, transWrapper)
	time.Sleep(50 * time.Millisecond)

	_, ok = actor.ProcessRegistry.GetLocal(conflictSetRouter.GetId() + "/" + string(conflictSetIDToInternalID([]byte(transWrapper.ConflictSetID))))
	assert.False(t, ok)
}

func TestHandlesDeadlocks(t *testing.T) {
	numSigners := 3
	ts := testnotarygroup.NewTestSet(t, numSigners)
	ng, err := newActorlessSystem(ts)
	require.Nil(t, err)

	rootContext := actor.EmptyRootContext

	sigGeneratorActors := make([]*actor.PID, numSigners)
	for i, signer := range ng.AllSigners() {
		sg, err := rootContext.SpawnNamed(NewSignatureGeneratorProps(signer, ng), "sigGenerator-"+signer.ID)
		require.Nil(t, err)
		sigGeneratorActors[i] = sg
		defer sg.Poison()
	}
	signer := ng.AllSigners()[0]

	alwaysChecker := rootContext.Spawn(NewAlwaysVerifierProps())
	defer alwaysChecker.Poison()

	sender := rootContext.Spawn(NewNullActorProps())
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
			context.Send(isReadyFuture.PID(), cs)
		case *messages.CurrentStateWrapper:
			context.Send(fut.PID(), msg)
		}
	}

	parent, err := rootContext.SpawnNamed(actor.PropsFromFunc(parentFunc), "THDParent")
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
		rootContext.Send(conflictSetRouter, trans[i])
	}
	// it's known that trans[0] is the lowest transaction,
	// this is just a sanity check
	require.True(t, string(trans[1].TransactionID) < string(trans[2].TransactionID))
	require.True(t, string(trans[1].TransactionID) < string(trans[0].TransactionID))

	// note skipping first signer here
	for i := 1; i < len(sigGeneratorActors); i++ {
		sig, err := rootContext.RequestFuture(sigGeneratorActors[i], trans[i], 1*time.Second).Result()
		require.Nil(t, err)
		rootContext.Send(conflictSetRouter, sig)
	}

	// at this point the first signer should have 3 transactions with 1 signature each and be in a deadlocked state
	// which means it should sign the lowest transaction (a different one than it did before)
	// one more signature on that same transaction should get it to quorum in the new view
	sig, err := rootContext.RequestFuture(sigGeneratorActors[1], trans[1], 1*time.Second).Result()
	require.Nil(t, err)
	rootContext.Send(conflictSetRouter, sig)

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

	rootContext := actor.EmptyRootContext

	sigGeneratorActors := make([]*actor.PID, numSigners)
	for i, signer := range ng.AllSigners() {
		sg, err := rootContext.SpawnNamed(NewSignatureGeneratorProps(signer, ng), "sigGenerator-"+signer.ID)
		require.Nil(t, err)
		sigGeneratorActors[i] = sg
		defer sg.Poison()
	}
	signer := ng.AllSigners()[0]

	alwaysChecker := rootContext.Spawn(NewAlwaysVerifierProps())
	defer alwaysChecker.Poison()

	sender := rootContext.Spawn(NewNullActorProps())
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
			context.Send(isReadyFuture0.PID(), cs)
		case *messages.CurrentStateWrapper:
			context.Send(fut0.PID(), msg)
		}
	}

	parent0, err := rootContext.SpawnNamed(actor.PropsFromFunc(parentFunc0), "THCBTParent0")
	require.Nil(t, err)
	defer parent0.Poison()

	csInterface0, err := isReadyFuture0.Result()
	require.Nil(t, err)
	conflictSetRouter0 := csInterface0.(*actor.PID)

	trans := testhelpers.NewValidTransaction(t)
	transWrapper := fakeValidateTransaction(t, &trans)

	rootContext.Send(conflictSetRouter0, transWrapper)

	// note skipping first signer here
	for i := 1; i < len(sigGeneratorActors); i++ {
		sig, err := rootContext.RequestFuture(sigGeneratorActors[i], transWrapper, 1*time.Second).Result()
		require.Nil(t, err)
		rootContext.Send(conflictSetRouter0, sig)
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
			context.Send(isReadyFuture1.PID(), cs)
		case *messages.CurrentStateWrapper:
			context.Send(fut1.PID(), msg)
		}
	}

	parent1, err := rootContext.SpawnNamed(actor.PropsFromFunc(parentFunc1), "THCBTParent1")
	require.Nil(t, err)
	defer parent1.Poison()

	csInterface1, err := isReadyFuture1.Result()
	require.Nil(t, err)
	conflictSetRouter1 := csInterface1.(*actor.PID)

	rootContext.Send(conflictSetRouter1, &commitNotification{
		objectID: trans.ObjectID,
		store: &extmsgs.Store{
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

var nullActorFunc = func(_ actor.Context) {}

func NewNullActorProps() *actor.Props {
	return actor.PropsFromFunc(nullActorFunc)
}
