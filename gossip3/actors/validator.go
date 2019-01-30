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
	cid "github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/chaintree/typecaster"
	"github.com/quorumcontrol/storage"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
)

type stateTransaction struct {
	ObjectID      []byte
	Transaction   []byte
	CurrentState  []byte
	TransactionID []byte
	ConflictSetID string
	payload       []byte
}

type TransactionValidator struct {
	middleware.LogAwareHolder
	reader storage.Reader
}

const maxValidatorConcurrency = 100

func NewTransactionValidatorProps(currentStateStore storage.Storage) *actor.Props {
	return router.NewRoundRobinPool(maxValidatorConcurrency).WithProducer(func() actor.Actor {
		return &TransactionValidator{
			reader: currentStateStore,
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (tv *TransactionValidator) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.Store:
		tv.Log.Debugw("stateHandler initial", "key", msg.Key)
		tv.handleStore(context, msg)
	}
}

func (tv *TransactionValidator) handleStore(context actor.Context, msg *messages.Store) {
	wrapper := &messages.TransactionWrapper{
		Key:      msg.Key,
		Value:    msg.Value,
		Accepted: false,
		Metadata: messages.MetadataMap{"seen": time.Now()},
	}
	var t messages.Transaction
	_, err := t.UnmarshalMsg(msg.Value)
	if err != nil {
		context.Respond(wrapper)
		return
	}
	wrapper.ConflictSetID = t.ConflictSetID()
	wrapper.Transaction = &t
	wrapper.TransactionID = msg.Key

	bits, err := tv.reader.Get(t.ObjectID)
	if err != nil {
		panic(fmt.Errorf("error getting current state: %v", err))
	}

	var currTip []byte
	if len(bits) > 0 {
		var currentState messages.CurrentState
		_, err := currentState.UnmarshalMsg(bits)
		if err != nil {
			panic(fmt.Sprintf("error unmarshaling: %v", err))
		}
		currTip = currentState.Signature.NewTip
	}

	if !bytes.Equal(crypto.Keccak256(msg.Value), msg.Key) {
		tv.Log.Errorw("invalid transaction: key did not match value")
		context.Respond(wrapper)
		return
	}

	st := &stateTransaction{
		ObjectID:      t.ObjectID,
		Transaction:   t.Payload,
		TransactionID: msg.Key,
		CurrentState:  currTip,
		ConflictSetID: wrapper.ConflictSetID,
		payload:       msg.Value,
	}

	nextState, accepted, err := chainTreeStateHandler(st)

	if accepted && bytes.Equal(nextState, t.NewTip) {
		tv.Log.Debugw("accepted", "key", msg.Key)
		wrapper.Accepted = true
		context.Respond(wrapper)
		return
	}

	context.Respond(wrapper)
}

func chainTreeStateHandler(stateTrans *stateTransaction) (nextState []byte, accepted bool, err error) {
	var currentTip cid.Cid
	if len(stateTrans.CurrentState) > 1 {
		currentTip, err = cid.Cast(stateTrans.CurrentState)
		if err != nil {
			return nil, false, fmt.Errorf("error casting CID: %v", err)
		}
	}

	addBlockrequest := &consensus.AddBlockRequest{}
	err = cbornode.DecodeInto(stateTrans.Transaction, addBlockrequest)
	if err != nil {
		return nil, false, fmt.Errorf("error getting payload: %v", err)
	}

	if currentTip.Defined() {
		if !currentTip.Equals(*addBlockrequest.Tip) {
			// log.Errorf("unmatching tips %s, %s", currentTip.String(), addBlockrequest.Tip.String())
			return nil, false, &consensus.ErrorCode{Memo: "unknown tip", Code: consensus.ErrInvalidTip}
		}
	} else {
		currentTip = *addBlockrequest.Tip
	}

	cborNodes := make([]*cbornode.Node, len(addBlockrequest.Nodes))

	sw := &safewrap.SafeWrap{}

	for i, node := range addBlockrequest.Nodes {
		cborNodes[i] = sw.Decode(node)
	}

	if sw.Err != nil {
		return nil, false, fmt.Errorf("error decoding: %v", sw.Err)
	}
	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())
	tree := dag.NewDag(currentTip, nodeStore)
	tree.AddNodes(cborNodes...)

	chainTree, err := chaintree.NewChainTree(
		tree,
		[]chaintree.BlockValidatorFunc{
			isOwner,
		},
		consensus.DefaultTransactors,
	)

	if err != nil {
		return nil, false, fmt.Errorf("error creating chaintree: %v", err)
	}

	isValid, err := chainTree.ProcessBlock(addBlockrequest.NewBlock)
	if !isValid || err != nil {
		return nil, false, fmt.Errorf("error processing: %v", err)
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
