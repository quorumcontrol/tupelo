package actors

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	cid "github.com/ipfs/go-cid"
	"github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/chaintree/typecaster"
	"github.com/quorumcontrol/tupelo/consensus"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/storage"
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
	currentStateActor *actor.PID
	reader            storage.Reader
}

func NewTransactionValidatorProps(currentState *actor.PID) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &TransactionValidator{
			currentStateActor: currentState,
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (tv *TransactionValidator) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *actor.Started:
		reader, err := tv.currentStateActor.RequestFuture(&messages.GetThreadsafeReader{}, 500*time.Millisecond).Result()
		if err != nil {
			panic(fmt.Sprintf("timeout waiting: %v", err))
		}
		tv.reader = reader.(storage.Reader)
	case *messages.Store:
		tv.Log.Debugw("stateHandler initial", "key", msg.Key)
		tv.handleStore(context, msg)
	}
}

// TODO: turn this into an actor so that we can scale it

func (tv *TransactionValidator) handleStore(context actor.Context, msg *messages.Store) {
	var t messages.Transaction
	_, err := t.UnmarshalMsg(msg.Value)
	if err != nil {
		context.Respond(&messages.TransactionWrapper{
			Key:      msg.Key,
			Value:    msg.Value,
			Accepted: false,
		})
		return
	}

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
		currTip = currentState.Tip
	}

	st := &stateTransaction{
		ObjectID:    t.ObjectID,
		Transaction: t.Payload,
		// TODO: verify transaction ID is correct
		TransactionID: msg.Key,
		CurrentState:  currTip,
		ConflictSetID: string(append(t.ObjectID, bits...)),
		//TODO: verify payload
		payload: msg.Value,
	}

	nextState, accepted, err := chainTreeStateHandler(st)
	if err != nil {
		panic(fmt.Sprintf("error validating chain tree"))
	}
	if accepted && bytes.Equal(nextState, t.NewTip) {
		context.Respond(&messages.TransactionWrapper{
			ConflictSetID: t.ConflictSetID(),
			Transaction:   &t,
			Key:           msg.Key,
			Value:         msg.Value,
			Accepted:      true,
		})
		return
	}

	context.Respond(&messages.TransactionWrapper{
		Transaction: &t,
		Key:         msg.Key,
		Value:       msg.Value,
		Accepted:    false,
	})
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
		var authentications []*consensus.PublicKey
		err = typecaster.ToType(uncastAuths, &authentications)
		if err != nil {
			return false, &consensus.ErrorCode{Code: consensus.ErrUnknown, Memo: fmt.Sprintf("err casting: %v", err)}
		}

		addrs = make([]string, len(authentications))
		for i, key := range authentications {
			addrs[i] = consensus.PublicKeyToAddr(key)
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
