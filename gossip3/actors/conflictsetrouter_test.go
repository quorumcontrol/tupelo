package actors

import (
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/quorumcontrol/tupelo/storage"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	datastore "github.com/ipfs/go-datastore"
	"github.com/quorumcontrol/messages/build/go/services"
	"github.com/quorumcontrol/tupelo-go-sdk/consensus"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/testhelpers"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
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
		defer rootContext.Poison(sg)
	}
	signer := ng.AllSigners()[0]

	alwaysChecker := rootContext.Spawn(NewAlwaysVerifierProps())
	defer rootContext.Poison(alwaysChecker)

	sender := rootContext.Spawn(NewNullActorProps())
	defer rootContext.Poison(sender)

	cfg := &ConflictSetRouterConfig{
		NotaryGroup:        ng,
		Signer:             signer,
		SignatureChecker:   alwaysChecker,
		SignatureSender:    sender,
		SignatureGenerator: sigGeneratorActors[0],
		CurrentStateStore:  storage.NewDefaultMemory(),
		PubSubSystem:       remote.NewSimulatedPubSub(),
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
	defer rootContext.Poison(parent)

	trans := testhelpers.NewValidTransaction(t)
	transWrapper := fakeValidateTransaction(t, &trans)

	_, err = isReadyFuture.Result()
	require.Nil(t, err)
	rootContext.Send(conflictSetRouter, transWrapper)

	// note skipping first signer here
	signTransaction(t, transWrapper, conflictSetRouter, sigGeneratorActors)

	msg, err := fut.Result()
	require.Nil(t, err)
	assert.True(t, msg.(*messages.CurrentStateWrapper).Verified)
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
		defer rootContext.Poison(sg)
	}
	signer := ng.AllSigners()[0]

	alwaysChecker := rootContext.Spawn(NewAlwaysVerifierProps())
	defer rootContext.Poison(alwaysChecker)

	sender := rootContext.Spawn(NewNullActorProps())
	defer rootContext.Poison(sender)

	cfg := &ConflictSetRouterConfig{
		NotaryGroup:        ng,
		Signer:             signer,
		SignatureChecker:   alwaysChecker,
		SignatureSender:    sender,
		SignatureGenerator: sigGeneratorActors[0],
		CurrentStateStore:  storage.NewDefaultMemory(),
		PubSubSystem:       remote.NewSimulatedPubSub(),
	}

	fut := actor.NewFuture(10 * time.Second)

	isReadyFuture := actor.NewFuture(5 * time.Second)

	var conflictSetRouter *actor.PID
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
	defer rootContext.Poison(parent)

	keyBytes, err := hexutil.Decode("0xf9c0b741e7c065ea4fe4fde335c4ee575141db93236e3d86bb1c9ae6ccddf6f1")
	require.Nil(t, err)
	treeKey, err := crypto.ToECDSA(keyBytes)
	require.Nil(t, err)

	csInterface, err := isReadyFuture.Result()
	require.Nil(t, err)
	conflictSetRouter = csInterface.(*actor.PID)

	trans := make([]*messages.TransactionWrapper, len(sigGeneratorActors))
	var conflictSetID string
	for i := 0; i < len(trans); i++ {
		tr := testhelpers.NewValidTransactionWithPathAndValue(t, treeKey, "path/to/somewhere", strconv.Itoa(i))
		trID := consensus.ConflictSetID(tr.ObjectId, tr.Height)
		require.Truef(t, conflictSetID == "" || trID == conflictSetID, "test transactions should all be in the same conflict set")
		conflictSetID = trID
		trans[i] = fakeValidateTransaction(t, &tr)
		rootContext.Send(conflictSetRouter, trans[i])
	}
	// it's known that trans[1] is the lowest transaction,
	// this is just a sanity check
	require.True(t, string(trans[1].TransactionId) < string(trans[2].TransactionId))
	require.True(t, string(trans[1].TransactionId) < string(trans[0].TransactionId))
	// [1,0,2] is the order

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
		defer rootContext.Poison(sg)
	}
	signer := ng.AllSigners()[0]

	alwaysChecker := rootContext.Spawn(NewAlwaysVerifierProps())
	defer rootContext.Poison(alwaysChecker)

	sender := rootContext.Spawn(NewNullActorProps())
	defer rootContext.Poison(sender)

	cfg := &ConflictSetRouterConfig{
		NotaryGroup:        ng,
		Signer:             signer,
		SignatureChecker:   alwaysChecker,
		SignatureSender:    sender,
		SignatureGenerator: sigGeneratorActors[0],
		CurrentStateStore:  storage.NewDefaultMemory(),
		PubSubSystem:       remote.NewSimulatedPubSub(),
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
	defer rootContext.Poison(parent0)

	csInterface0, err := isReadyFuture0.Result()
	require.Nil(t, err)
	conflictSetRouter0 := csInterface0.(*actor.PID)

	trans := testhelpers.NewValidTransaction(t)
	transWrapper := fakeValidateTransaction(t, &trans)

	rootContext.Send(conflictSetRouter0, transWrapper)

	// note skipping first signer here
	signTransaction(t, transWrapper, conflictSetRouter0, sigGeneratorActors)

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
	defer rootContext.Poison(parent1)

	csInterface1, err := isReadyFuture1.Result()
	require.Nil(t, err)
	conflictSetRouter1 := csInterface1.(*actor.PID)

	rootContext.Send(conflictSetRouter1, currentStateWrapper.CurrentState)

	msg, err = fut1.Result()
	require.Nil(t, err)
	wrap := msg.(*messages.CurrentStateWrapper)
	assert.True(t, wrap.Verified)
	assert.Equal(t, trans.NewTip, wrap.CurrentState.Signature.NewTip)
}

func fakeValidateTransaction(t testing.TB, trans *services.AddBlockRequest) *messages.TransactionWrapper {
	wrapper := &messages.TransactionWrapper{
		TransactionId: consensus.RequestID(trans),
		Transaction:   trans,
		ConflictSetID: consensus.ConflictSetID(trans.ObjectId, trans.Height),
		PreFlight:     true,
		Accepted:      true,
		Metadata:      messages.MetadataMap{"seen": time.Now()},
	}
	wrapper.StartTrace("transaction")
	return wrapper
}

func newActorlessSystem(testSet *testnotarygroup.TestSet) (*types.NotaryGroup, error) {
	ng := types.NewNotaryGroup("actorless")
	for i, signKey := range testSet.SignKeys {
		sk := signKey
		signer := types.NewLocalSigner(consensus.PublicKeyToEcdsaPub(&testSet.PubKeys[i]), sk)
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

func spawnCSR(t *testing.T, prefix string, i int, currentStateStore datastore.Read,
	ng *types.NotaryGroup, checker, sender *actor.PID, sigGenerators []*actor.PID) (
	chan *messages.CurrentStateWrapper, *actor.PID, *actor.PID) {
	ctx := actor.EmptyRootContext

	signer := ng.AllSigners()[0]
	cfg := &ConflictSetRouterConfig{
		NotaryGroup:        ng,
		Signer:             signer,
		SignatureChecker:   checker,
		SignatureSender:    sender,
		SignatureGenerator: sigGenerators[0],
		CurrentStateStore:  currentStateStore,
		PubSubSystem:       remote.NewSimulatedPubSub(),
	}

	cswChan := make(chan *messages.CurrentStateWrapper)
	csrChan := make(chan *actor.PID)
	parentName := fmt.Sprintf("%sParent%d", prefix, i)
	t.Logf("starting conflict set router parent %s", parentName)
	parent, err := ctx.SpawnNamed(actor.PropsFromFunc(func(actorCtx actor.Context) {
		switch msg := actorCtx.Message().(type) {
		case *actor.Started:
			routerName := fmt.Sprintf("%sCSR%d", prefix, i)
			t.Logf("starting conflict set router %s", routerName)
			cs, err := actorCtx.SpawnNamed(NewConflictSetRouterProps(cfg), routerName)
			require.Nil(t, err)
			csrChan <- cs
		case *messages.CurrentStateWrapper:
			cswChan <- msg
		}
	}), parentName)
	require.Nil(t, err)
	conflictSetRouter := <-csrChan

	return cswChan, conflictSetRouter, parent
}

func signTransaction(t *testing.T, transWrapper *messages.TransactionWrapper,
	csr *actor.PID, sigGeneratorActors []*actor.PID) {
	ctx := actor.EmptyRootContext
	// note skipping first signer here
	for i := 1; i < len(sigGeneratorActors); i++ {
		sig, err := ctx.RequestFuture(sigGeneratorActors[i], transWrapper, 1*time.Second).Result()
		require.Nil(t, err)
		ctx.Send(csr, sig)
	}
}

// Test that ConflictSetRouter cleans up stale conflict sets once it receives a commit
// notification.
func TestCleansUpStaleConflictSetsOnCommit(t *testing.T) {
	ctx := actor.EmptyRootContext
	numSigners := 3
	ts := testnotarygroup.NewTestSet(t, numSigners)
	ng, err := newActorlessSystem(ts)
	require.Nil(t, err)

	sigGeneratorActors := make([]*actor.PID, numSigners)
	for i, signer := range ng.AllSigners() {
		sg, err := actor.SpawnNamed(NewSignatureGeneratorProps(signer, ng), "sigGenerator-"+signer.ID)
		require.Nil(t, err)
		sigGeneratorActors[i] = sg
		defer ctx.Poison(sg)
	}

	alwaysChecker := actor.Spawn(NewAlwaysVerifierProps())
	defer ctx.Poison(alwaysChecker)

	sender := actor.Spawn(NewNullActorProps())
	defer ctx.Poison(sender)

	currentStateStore := storage.NewDefaultMemory()

	cswChan0, conflictSetRouter0, parent0 := spawnCSR(t, "testCUSCSOC", 0, currentStateStore, ng,
		alwaysChecker,
		sender, sigGeneratorActors)
	defer ctx.Poison(parent0)

	cswChan1, conflictSetRouter1, parent1 := spawnCSR(t, "testCUSCSOC", 1, currentStateStore, ng,
		alwaysChecker, sender, sigGeneratorActors)
	defer ctx.Poison(parent1)

	trans0 := testhelpers.NewValidTransaction(t)
	trans0.Height = 1
	transWrapper0 := fakeValidateTransaction(t, &trans0)
	t.Logf("sending transaction wrapper #1 to conflict set router #1")
	ctx.Send(conflictSetRouter0, transWrapper0)
	signTransaction(t, transWrapper0, conflictSetRouter0, sigGeneratorActors)

	t.Log("waiting for current state wrapper #1 from parent actor #1")
	currentStateWrapper0 := <-cswChan0
	t.Logf("got current state wrapper #1 from parent actor #1: %+v",
		currentStateWrapper0.CurrentState.Signature)
	require.True(t, currentStateWrapper0.Verified)
	require.Equal(t, trans0.NewTip, currentStateWrapper0.CurrentState.Signature.NewTip)
	require.Equal(t, uint64(1), currentStateWrapper0.CurrentState.Signature.Height)

	// Store current state in store so that second CSR knows the current state height
	t.Logf("storing current state in store, ObjectId: %s", trans0.ObjectId)
	err = currentStateStore.Put(datastore.NewKey(string(trans0.ObjectId)), currentStateWrapper0.MustMarshal())
	require.Nil(t, err)

	// Send a transaction to first CSR, to get stale on receipt of the commit notification
	trans1 := testhelpers.NewValidTransaction(t)
	trans1.ObjectId = trans0.ObjectId
	trans1.Height = 2
	transWrapper1 := fakeValidateTransaction(t, &trans1)
	t.Logf("sending transaction wrapper #2 to conflict set router #1")
	ctx.Send(conflictSetRouter0, transWrapper1)

	// Send another transaction to first CSR, to get stale on receipt of the commit notification
	trans2 := testhelpers.NewValidTransaction(t)
	trans2.ObjectId = trans0.ObjectId
	trans2.Height = 3
	transWrapper2 := fakeValidateTransaction(t, &trans2)
	t.Logf("sending transaction wrapper #3 to conflict set router #1")
	ctx.Send(conflictSetRouter0, transWrapper2)

	// Send a transaction to second CSR, which forms basis for commit notification to first CSR
	trans3 := testhelpers.NewValidTransaction(t)
	trans3.ObjectId = trans0.ObjectId
	trans3.Height = 3
	transWrapper3 := fakeValidateTransaction(t, &trans3)
	require.Equal(t, transWrapper2.ConflictSetID, transWrapper3.ConflictSetID)
	ctx.Send(conflictSetRouter1, transWrapper3)
	signTransaction(t, transWrapper3, conflictSetRouter1, sigGeneratorActors)

	t.Log("waiting for current state wrapper #2 from parent actor #2")
	currentStateWrapper1 := <-cswChan1
	t.Logf("got current state wrapper #2 from parent actor #2")
	require.True(t, currentStateWrapper1.Verified)
	require.Equal(t, trans3.NewTip, currentStateWrapper1.CurrentState.Signature.NewTip)
	require.Equal(t, uint64(trans3.Height), currentStateWrapper1.CurrentState.Signature.Height)
	require.Equal(t, trans0.ObjectId, currentStateWrapper1.CurrentState.Signature.ObjectId)

	t.Logf("sending commit notification to conflict set router #1")
	ctx.Send(conflictSetRouter0, currentStateWrapper1.CurrentState)

	t.Log("waiting for current state wrapper #3 from parent actor #1")
	currentStateWrapper2 := <-cswChan0
	t.Logf("got current state wrapper #3 from parent actor #1")
	require.True(t, currentStateWrapper2.Verified)
	require.Equal(t, uint64(trans3.Height), currentStateWrapper2.CurrentState.Signature.Height)
	require.Equal(t, trans3.NewTip, currentStateWrapper2.CurrentState.Signature.NewTip)

	numConflictSets, err := ctx.RequestFuture(conflictSetRouter0,
		messages.GetNumConflictSets{}, 1*time.Second).Result()
	require.Nil(t, err)
	require.Equal(t, 0, numConflictSets)
}
