package gossip4

import (
	"context"
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	format "github.com/ipfs/go-ipld-format"

	peer "github.com/libp2p/go-libp2p-peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	sigfuncs "github.com/quorumcontrol/tupelo-go-sdk/signatures"

	"github.com/quorumcontrol/messages/build/go/services"
	"github.com/quorumcontrol/messages/build/go/signatures"
	"github.com/quorumcontrol/tupelo-go-sdk/consensus"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
)

type transactionValidator struct {
	group      *types.NotaryGroup
	validators []chaintree.BlockValidatorFunc
	node       *actor.PID
}

func (tv *transactionValidator) setup() error {
	validators, err := blockValidators(tv.group)
	tv.validators = validators
	return err
}

func blockValidators(group *types.NotaryGroup) ([]chaintree.BlockValidatorFunc, error) {
	quorumCount := group.QuorumCount()
	signers := group.AllSigners()
	verKeys := make([]*bls.VerKey, len(signers))
	for i, signer := range signers {
		verKeys[i] = signer.VerKey
	}

	sigVerifier := types.GenerateIsValidSignature(func(state *signatures.TreeState) (bool, error) {
		if uint64(sigfuncs.SignerCount(state.Signature)) < quorumCount {
			return false, nil
		}

		return verifySignature(
			context.TODO(),
			consensus.GetSignable(state),
			state.Signature,
			verKeys,
		)
	})

	blockValidators, err := group.BlockValidators(context.TODO())
	if err != nil {
		return nil, fmt.Errorf("error getting block validators: %v", err)
	}
	return append(blockValidators, sigVerifier), nil
}

func (tv *transactionValidator) validate(ctx context.Context, pID peer.ID, msg *pubsub.Message) bool {
	abr, err := pubsubMsgToAddBlockRequest(ctx, msg)
	if err != nil {
		logger.Errorf("error converting message to abr: %v", err)
		return false
	}
	block := &chaintree.BlockWithHeaders{}
	err = cbornode.DecodeInto(abr.Payload, block)
	if err != nil {
		logger.Errorf("invalid transaction: payload is not a block: %v", err)
		return false
	}

	cborNodes := make([]format.Node, len(abr.State))

	sw := &safewrap.SafeWrap{}

	for i, node := range abr.State {
		cborNodes[i] = sw.Decode(node)
	}
	if sw.Err != nil {
		logger.Errorf("error decoding (nodes: %d): %v", len(cborNodes), sw.Err)
		return false
	}

	nodeStore := nodestore.MustMemoryStore(ctx)

	transPreviousTip, err := cid.Cast(abr.PreviousTip)
	if err != nil {
		logger.Errorf("error casting CID: %v", err)
		return false
	}

	tree := dag.NewDag(ctx, transPreviousTip, nodeStore)

	chainTree, err := chaintree.NewChainTree(
		ctx,
		tree,
		tv.validators,
		tv.group.Config().Transactions,
	)

	if err != nil {
		logger.Errorf("error creating chaintree (tip: %s, nodes: %d): %v", transPreviousTip.String(), len(cborNodes), err)
		return false
	}

	isValid, err := chainTree.ProcessBlock(ctx, block)
	if !isValid || err != nil {
		var errMsg string
		if err == nil {
			errMsg = "invalid transaction"
		} else {
			errMsg = err.Error()
		}
		logger.Errorf("error processing: %v", errMsg)
		return false
	}

	// we do something a bit odd here and send the ABR through an actor notification rather
	// then just letting a pubsub subscribe happen, because we've already done the decoding work.

	actor.EmptyRootContext.Send(tv.node, abr)

	return true
}

func pubsubMsgToAddBlockRequest(ctx context.Context, msg *pubsub.Message) (*services.AddBlockRequest, error) {
	abr := &services.AddBlockRequest{}
	err := abr.Unmarshal(msg.Data)
	return abr, err
}
