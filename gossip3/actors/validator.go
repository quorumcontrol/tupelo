package actors

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/AsynkronIT/protoactor-go/router"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/chaintree/typecaster"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo-go-client/consensus"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
)

type stateTransaction struct {
	ObjectID      []byte
	Transaction   *extmsgs.Transaction
	CurrentState  []byte
	TransactionID []byte
	ConflictSetID string
	Block         *chaintree.BlockWithHeaders
	payload       []byte
}

type TransactionValidator struct {
	middleware.LogAwareHolder
	reader storage.Reader
}

type validationRequest struct {
	key   []byte
	value []byte
}

const maxValidatorConcurrency = 10

func NewTransactionValidatorProps(currentStateStore storage.Reader) *actor.Props {
	return router.NewRoundRobinPool(maxValidatorConcurrency).WithProducer(func() actor.Actor {
		return &TransactionValidator{
			reader: currentStateStore,
		}
	}).WithReceiverMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (tv *TransactionValidator) Receive(actorCtx actor.Context) {
	switch msg := actorCtx.Message().(type) {
	case *validationRequest:
		tv.Log.Debugw("stateHandler initial", "key", msg.key)
		tv.handleRequest(actorCtx, msg)
	}
}

func (tv *TransactionValidator) nextHeight(objectID []byte) uint64 {
	return nextHeight(tv.reader, objectID)
}

func (tv *TransactionValidator) handleRequest(actorCtx actor.Context, msg *validationRequest) {
	wrapper := &messages.TransactionWrapper{
		Key:       msg.key,
		Value:     msg.value,
		PreFlight: false,
		Accepted:  false,
		Stale:     false,
		Metadata:  messages.MetadataMap{"seen": time.Now()},
	}
	parentSpan := wrapper.StartTrace()

	sp := wrapper.NewSpan("validator")
	defer sp.Finish()

	var t extmsgs.Transaction
	_, err := t.UnmarshalMsg(msg.value)
	if err != nil {
		tv.Log.Infow("error unmarshaling", "err", err)
		actorCtx.Respond(wrapper)
		return
	}
	wrapper.ConflictSetID = t.ConflictSetID()
	wrapper.Transaction = &t
	wrapper.TransactionID = msg.key
	parentSpan.SetTag("conflictSetID", string(wrapper.ConflictSetID))
	parentSpan.SetTag("transactionID", string(wrapper.TransactionID))

	var currTip []byte
	objectIDBits, err := tv.reader.Get(t.ObjectID)
	if err != nil {
		panic(fmt.Errorf("error getting current state: %v", err))
	}

	if len(objectIDBits) > 0 {
		expectedHeight := tv.nextHeight(t.ObjectID)
		var currentState extmsgs.CurrentState
		_, err := currentState.UnmarshalMsg(objectIDBits)
		if err != nil {
			panic(fmt.Sprintf("error unmarshaling: %v", err))
		}
		if expectedHeight == t.Height {
			currTip = currentState.Signature.NewTip
		} else if expectedHeight < t.Height {
			wrapper.PreFlight = true
			actorCtx.Respond(wrapper)
			return
		} else {
			tv.Log.Debugf("transaction height %d is lower than current state height %d; ignoring", t.Height, expectedHeight)
			wrapper.Stale = true
			actorCtx.Respond(wrapper)
			return
		}
	} else {
		if t.Height != 0 {
			wrapper.PreFlight = true
			actorCtx.Respond(wrapper)
			return
		}
	}

	if !bytes.Equal(crypto.Keccak256(msg.value), msg.key) {
		tv.Log.Errorw("invalid transaction: key did not match value")
		actorCtx.Respond(wrapper)
		return
	}

	block := &chaintree.BlockWithHeaders{}
	err = cbornode.DecodeInto(t.Payload, block)
	if err != nil {
		tv.Log.Errorw("invalid transaction: payload is not a block")
		actorCtx.Respond(wrapper)
		return
	}

	if block.Height != t.Height {
		tv.Log.Errorw("invalid transaction block height != transaction height", "blockHeight", block.Height, "transHeight", t.Height, "transaction", msg.key)
		actorCtx.Respond(wrapper)
		return
	}

	st := &stateTransaction{
		ObjectID:      t.ObjectID,
		Transaction:   &t,
		TransactionID: msg.key,
		CurrentState:  currTip,
		ConflictSetID: wrapper.ConflictSetID,
		Block:         block,
		payload:       msg.value,
	}

	nextState, accepted, err := chainTreeStateHandler(st)

	expectedNewTip := bytes.Equal(nextState, t.NewTip)
	if accepted && expectedNewTip {
		tv.Log.Debugw("accepted", "key", msg.key)
		wrapper.Accepted = true
		actorCtx.Respond(wrapper)
		return
	}
	sp.LogKV("accepted", false)

	if err == nil && !expectedNewTip {
		nextStateCid, _ := cid.Cast(nextState)
		newTipCid, _ := cid.Cast(t.NewTip)
		err = fmt.Errorf("error: expected new tip: %s but got: %s", nextStateCid.String(), newTipCid.String())
	}
	sp.LogKV("error", err)
	wrapper.Metadata["error"] = err

	tv.Log.Debugw("rejected", "err", err)

	actorCtx.Respond(wrapper)
}

func chainTreeStateHandler(stateTrans *stateTransaction) (nextState []byte, accepted bool, err error) {
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

	if currentTip.Defined() {
		if !currentTip.Equals(transPreviousTip) {
			return nil, false, &consensus.ErrorCode{Memo: "unknown tip", Code: consensus.ErrInvalidTip}
		}
	}

	cborNodes := make([]*cbornode.Node, len(stateTrans.Transaction.State))

	sw := &safewrap.SafeWrap{}

	for i, node := range stateTrans.Transaction.State {
		cborNodes[i] = sw.Decode(node)
	}
	if sw.Err != nil {
		return nil, false, fmt.Errorf("error decoding (nodes: %d): %v", len(cborNodes), sw.Err)
	}

	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())

	var tree *dag.Dag

	if currentTip.Defined() {
		tree = dag.NewDag(currentTip, nodeStore)
	} else {
		tree = consensus.NewEmptyTree(string(stateTrans.ObjectID), nodeStore)
	}

	if err = tree.AddNodes(cborNodes...); err != nil {
		return nil, false, err
	}

	chainTree, err := chaintree.NewChainTree(
		tree,
		[]chaintree.BlockValidatorFunc{
			isOwner,
		},
		consensus.DefaultTransactors,
	)

	if err != nil {
		return nil, false, fmt.Errorf("error creating chaintree (tip: %s, nodes: %d): %v", currentTip.String(), len(cborNodes), err)
	}

	isValid, err := chainTree.ProcessBlock(stateTrans.Block)
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

func isOwner(tree *dag.Dag, blockWithHeaders *chaintree.BlockWithHeaders) (bool, chaintree.CodedError) {

	id, _, err := tree.Resolve([]string{"id"})
	if err != nil {
		return false, &consensus.ErrorCode{Memo: fmt.Sprintf("error: %v", err), Code: consensus.ErrUnknown}
	}

	headers := &consensus.StandardHeaders{}

	err = typecaster.ToType(blockWithHeaders.Headers, headers)
	if err != nil {
		return false, &consensus.ErrorCode{Memo: fmt.Sprintf("error: %v", err), Code: consensus.ErrUnknown}
	}

	var addrs []string

	uncastAuths, _, err := tree.Resolve(strings.Split("tree/"+consensus.TreePathForAuthentications, "/"))
	if err != nil {
		return false, &consensus.ErrorCode{Code: consensus.ErrUnknown, Memo: fmt.Sprintf("err resolving: %v", err)}
	}
	// If there are no authentications then the Chain Tree is still owned by its genesis key
	if uncastAuths == nil {
		addrs = []string{consensus.DidToAddr(id.(string))}
	} else {
		err = typecaster.ToType(uncastAuths, &addrs)
		if err != nil {
			return false, &consensus.ErrorCode{Code: consensus.ErrUnknown, Memo: fmt.Sprintf("err casting: %v", err)}
		}
	}

	for _, addr := range addrs {
		isSigned, err := consensus.IsBlockSignedBy(blockWithHeaders, addr)

		if err != nil {
			return false, &consensus.ErrorCode{Memo: fmt.Sprintf("error finding if signed: %v", err), Code: consensus.ErrUnknown}
		}

		if isSigned {
			return true, nil
		}
	}

	return false, nil
}
