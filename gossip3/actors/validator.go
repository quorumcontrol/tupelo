package actors

import (
	"bytes"
	"context"
	"fmt"
	"time"

	datastore "github.com/ipfs/go-datastore"
	format "github.com/ipfs/go-ipld-format"

	"github.com/gogo/protobuf/proto"
	"github.com/quorumcontrol/messages/build/go/services"
	"github.com/quorumcontrol/messages/build/go/signatures"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/AsynkronIT/protoactor-go/router"
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/tupelo-go-sdk/consensus"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	sigfuncs "github.com/quorumcontrol/tupelo-go-sdk/signatures"

	"github.com/quorumcontrol/tupelo/gossip3/messages"
)

type stateTransaction struct {
	ObjectId      []byte
	Transaction   *services.AddBlockRequest
	CurrentState  []byte
	TransactionId []byte
	ConflictSetID string
	Block         *chaintree.BlockWithHeaders
}

type TransactionValidator struct {
	middleware.LogAwareHolder
	reader           datastore.Read
	signatureChecker *actor.PID
	notaryGroup      *types.NotaryGroup
	verKeys          []*bls.VerKey
	validators []chaintree.BlockValidatorFunc
}

type TransactionValidatorConfig struct {
	NotaryGroup       *types.NotaryGroup
	SignatureChecker  *actor.PID
	CurrentStateStore datastore.Read
}

type validationRequest struct {
	transaction *services.AddBlockRequest
}

const maxValidatorConcurrency = 10

func NewTransactionValidatorProps(cfg *TransactionValidatorConfig) *actor.Props {
	signers := cfg.NotaryGroup.AllSigners()
	verKeys := make([]*bls.VerKey, len(signers))
	for i, signer := range signers {
		verKeys[i] = signer.VerKey
	}

	return router.NewRoundRobinPool(maxValidatorConcurrency).WithProducer(func() actor.Actor {
		return &TransactionValidator{
			reader:           cfg.CurrentStateStore,
			signatureChecker: cfg.SignatureChecker,
			notaryGroup:      cfg.NotaryGroup,
			verKeys:          verKeys,
		}
	}).WithReceiverMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (tv *TransactionValidator) Receive(actorCtx actor.Context) {
	switch msg := actorCtx.Message().(type) {
	case *actor.Started:
		err := tv.setupBlockValidators(actorCtx)
		if err != nil {
			panic(err)
		}
	case *validationRequest:
		tv.Log.Debugw("transactionValidator initial", "key", msg.transaction.ObjectId)
		tv.handleRequest(actorCtx, msg)
	}
}

func (tv *TransactionValidator) setupBlockValidators(actorCtx actor.Context) error {
	quorumCount := tv.notaryGroup.QuorumCount()

	sigVerifier := types.GenerateIsValidSignature(func(state *signatures.TreeState) (bool, error) {

		if uint64(sigfuncs.SignerCount(state.Signature)) < quorumCount {
			return false, nil
		}

		resp, err := actorCtx.RequestFuture(tv.signatureChecker, &messages.SignatureVerification{
			Message:   consensus.GetSignable(state),
			Signature: state.Signature,
			VerKeys:   tv.verKeys,
		}, 2*time.Second).Result()
		if err != nil {
			return false, fmt.Errorf("error verifying signature: %v", err)
		}

		sigResult, ok := resp.(*messages.SignatureVerification)
		if !ok {
			return false, fmt.Errorf("error casting sig verify response; was a %T, expected a SignatureVerification", resp)
		}

		return sigResult.Verified, nil
	})

	blockValidators, err := tv.notaryGroup.BlockValidators(context.TODO())
	if err != nil {
		return fmt.Errorf("error getting block validators: %v", err)
	}
	tv.validators = append(blockValidators, sigVerifier)
	return nil
}

func (tv *TransactionValidator) nextHeight(ObjectId []byte) uint64 {
	return nextHeight(tv.Log, tv.reader, ObjectId)
}

func (tv *TransactionValidator) handleRequest(actorCtx actor.Context, msg *validationRequest) {
	t := msg.transaction

	tv.Log.Debugw("handling validation request")
	wrapper := &messages.TransactionWrapper{
		Transaction: t,
		PreFlight:   false,
		Accepted:    false,
		Stale:       false,
		Metadata:    messages.MetadataMap{"seen": time.Now()},
	}
	parentSpan := wrapper.StartTrace("transaction")

	sp := wrapper.NewSpan("validator")
	defer sp.Finish()

	wrapper.ConflictSetID = consensus.ConflictSetID(t.ObjectId, t.Height)
	wrapper.TransactionId = consensus.RequestID(t)
	parentSpan.SetTag("conflictSetID", string(wrapper.ConflictSetID))
	parentSpan.SetTag("TransactionId", string(wrapper.TransactionId))

	var currTip []byte
	objectIDBits, err := tv.reader.Get(datastore.NewKey(string(t.ObjectId)))
	if err != nil && err != datastore.ErrNotFound {
		panic(fmt.Errorf("error getting current state: %v", err))
	}

	if len(objectIDBits) > 0 {
		expectedHeight := tv.nextHeight(t.ObjectId)
		currentState := &signatures.TreeState{}
		err := proto.Unmarshal(objectIDBits, currentState)
		if err != nil {
			panic(fmt.Sprintf("error unmarshaling: %v", err))
		}
		if expectedHeight == t.Height {
			tv.Log.Debugw("transaction is for expected height")
			currTip = currentState.NewTip
		} else if expectedHeight < t.Height {
			tv.Log.Debugw("transaction has higher than expected height, marking as preflight",
				"height", t.Height, "expectedHeight", expectedHeight)
			wrapper.PreFlight = true
			actorCtx.Respond(wrapper)
			return
		} else {
			tv.Log.Debugf("transaction height %d is lower than current state height %d; ignoring", t.Height, expectedHeight)
			wrapper.Stale = true
			actorCtx.Respond(wrapper)
			return
		}
	} else if t.Height > 0 {
		tv.Log.Debugw("object corresponding to transaction not found, marking as preflight",
			"height", t.Height)
		wrapper.PreFlight = true
		actorCtx.Respond(wrapper)
		return
	}

	block := &chaintree.BlockWithHeaders{}
	err = cbornode.DecodeInto(t.Payload, block)
	if err != nil {
		tv.Log.Errorw("invalid transaction: payload is not a block", "err", err)
		actorCtx.Respond(wrapper)
		return
	}

	if block.Height != t.Height {
		tv.Log.Errorw("invalid transaction block height != transaction height", "blockHeight", block.Height, "transHeight", t.Height, "transaction", wrapper.TransactionId)
		actorCtx.Respond(wrapper)
		return
	}

	st := &stateTransaction{
		ObjectId:      t.ObjectId,
		Transaction:   t,
		TransactionId: wrapper.TransactionId,
		CurrentState:  currTip,
		ConflictSetID: wrapper.ConflictSetID,
		Block:         block,
	}

	nextState, accepted, err := tv.chainTreeStateHandler(actorCtx, st)

	expectedNewTip := bytes.Equal(nextState, t.NewTip)
	if accepted && expectedNewTip {
		tv.Log.Debugw("accepted", "key", wrapper.TransactionId)
		wrapper.Accepted = true
		sp.SetTag("accepted", true)
		actorCtx.Respond(wrapper)
		return
	}
	sp.SetTag("accepted", false)

	if err == nil && !expectedNewTip {
		nextStateCid, _ := cid.Cast(nextState)
		newTipCid, _ := cid.Cast(t.NewTip)
		err = fmt.Errorf("error: expected new tip: %s but got: %s", nextStateCid.String(), newTipCid.String())
	}
	sp.SetTag("error", err)
	wrapper.Metadata["error"] = err

	tv.Log.Debugw("rejected", "err", err)

	actorCtx.Respond(wrapper)
}

func (tv *TransactionValidator) chainTreeStateHandler(actorCtx actor.Context, stateTrans *stateTransaction) (nextState []byte, accepted bool, err error) {
	ctx := context.TODO()

	var currentTip cid.Cid
	if len(stateTrans.CurrentState) > 1 {
		currentTip, err = cid.Cast(stateTrans.CurrentState)
		if err != nil {
			return nil, false, fmt.Errorf("error casting CID: %v", err)
		}
	}

	var transPreviousTip cid.Cid
	transPreviousTip, err = cid.Cast(stateTrans.Transaction.PreviousTip)
	if err != nil {
		return nil, false, fmt.Errorf("error casting CID: %v", err)
	}

	if currentTip.Defined() && !currentTip.Equals(transPreviousTip) {
		return nil, false, &consensus.ErrorCode{Memo: "unknown tip", Code: consensus.ErrInvalidTip}
	}

	cborNodes := make([]format.Node, len(stateTrans.Transaction.State))

	sw := &safewrap.SafeWrap{}

	for i, node := range stateTrans.Transaction.State {
		cborNodes[i] = sw.Decode(node)
	}
	if sw.Err != nil {
		return nil, false, fmt.Errorf("error decoding (nodes: %d): %v", len(cborNodes), sw.Err)
	}

	nodeStore := nodestore.MustMemoryStore(ctx)

	var tree *dag.Dag

	if currentTip.Defined() {
		tree = dag.NewDag(ctx, currentTip, nodeStore)
	} else {
		tree = consensus.NewEmptyTree(string(stateTrans.ObjectId), nodeStore)
	}

	if err = tree.AddNodes(ctx, cborNodes...); err != nil {
		return nil, false, err
	}

	
	chainTree, err := chaintree.NewChainTree(
		ctx,
		tree,
		tv.validators,
		tv.notaryGroup.Config().Transactions,
	)

	if err != nil {
		return nil, false, fmt.Errorf("error creating chaintree (tip: %s, nodes: %d): %v", currentTip.String(), len(cborNodes), err)
	}

	isValid, err := chainTree.ProcessBlock(ctx, stateTrans.Block)
	if !isValid || err != nil {
		var errMsg string
		if err == nil {
			errMsg = "invalid transaction"
		} else {
			errMsg = err.Error()
		}
		return nil, false, fmt.Errorf("error processing: %v", errMsg)
	}

	return chainTree.Dag.Tip.Bytes(), true, nil
}
