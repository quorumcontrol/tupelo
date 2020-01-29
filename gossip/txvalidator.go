package gossip

import (
	"context"
	"fmt"

	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/opentracing/opentracing-go"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ipfs/go-cid"
	cbornode "github.com/ipfs/go-ipld-cbor"
	format "github.com/ipfs/go-ipld-format"
	logging "github.com/ipfs/go-log"

	pubsub "github.com/libp2p/go-libp2p-pubsub"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/tupelo-go-sdk/proof"

	"github.com/quorumcontrol/messages/v2/build/go/gossip"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo-go-sdk/consensus"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/types"
)

// TODO: Delete me when done with gossip 3 dark traffic testing!
var MagicValidNewTip = "convertedFromGossip3"

// TransactionValidator validates incoming pubsub messages for internal consistency
// and sends them to the gossip4 node. It is exported for the gossip3to4 module
type TransactionValidator struct {
	group      *types.NotaryGroup
	validators []chaintree.BlockValidatorFunc
	node       *actor.PID
	logger     logging.EventLogger
}

// NewTransactionValidator creates a new TransactionValidator
func NewTransactionValidator(ctx context.Context, logger logging.EventLogger, group *types.NotaryGroup, node *actor.PID) (*TransactionValidator, error) {
	tv := &TransactionValidator{
		group:  group,
		node:   node,
		logger: logger,
	}
	err := tv.setup(ctx)
	if err != nil {
		return nil, fmt.Errorf("error setting up transaction validator: %v", err)
	}
	return tv, nil
}

func (tv *TransactionValidator) setup(ctx context.Context) error {
	validators, err := blockValidators(ctx, tv.group)
	tv.validators = validators
	return err
}

func blockValidators(ctx context.Context, group *types.NotaryGroup) ([]chaintree.BlockValidatorFunc, error) {
	quorumCount := group.QuorumCount()
	signers := group.AllSigners()
	verKeys := make([]*bls.VerKey, len(signers))
	for i, signer := range signers {
		verKeys[i] = signer.VerKey
	}

	proofVerifier := types.GenerateHasValidProof(func(prf *gossip.Proof) (bool, error) {
		return proof.Verify(ctx, prf, quorumCount, verKeys)
	})

	blockValidators, err := group.BlockValidators(ctx)
	if err != nil {
		return nil, fmt.Errorf("error getting block validators: %v", err)
	}
	return append(blockValidators, proofVerifier), nil
}

func (tv *TransactionValidator) validate(ctx context.Context, pID peer.ID, msg *pubsub.Message) bool {
	wrapper := &AddBlockWrapper{}
	wrapper.StartTrace("gossip4.transaction")

	abr, err := pubsubMsgToAddBlockRequest(wrapper.GetContext(), msg)
	if err != nil {
		tv.logger.Errorf("error converting message to abr: %v", err)
		return false
	}
	validated := tv.ValidateAbr(wrapper.GetContext(), abr)
	if validated {
		// we do something a bit odd here and send the ABR through an actor notification rather
		// then just letting a pubsub subscribe happen, because we've already done the decoding work.
		wrapper.AddBlockRequest = abr
		wrapper.SetTag("valid", true)
		actor.EmptyRootContext.Send(tv.node, wrapper)
		return true
	}
	wrapper.SetTag("valid", false)
	wrapper.StopTrace()

	return false
}

func (tv *TransactionValidator) ValidateAbr(validateCtx context.Context, abr *services.AddBlockRequest) bool {
	sp, ctx := opentracing.StartSpanFromContext(validateCtx, "gossip4.validateABR")
	defer sp.Finish()

	transPreviousTip, err := cid.Cast(abr.PreviousTip)
	if err != nil {
		tv.logger.Errorf("error casting CID: %v", err)
		return false
	}

	block := &chaintree.BlockWithHeaders{}
	err = cbornode.DecodeInto(abr.Payload, block)
	if err != nil {
		tv.logger.Errorf("invalid transaction: payload is not a block: %v", err)
		return false
	}

	sw := &safewrap.SafeWrap{}
	cborNodes := make([]format.Node, len(abr.State))
	for i, node := range abr.State {
		cborNodes[i] = sw.Decode(node)
	}
	if sw.Err != nil {
		tv.logger.Errorf("error decoding (nodes: %d): %v", len(cborNodes), sw.Err)
		return false
	}

	nodeStore := nodestore.MustMemoryStore(ctx)
	err = nodeStore.AddMany(ctx, cborNodes)
	if err != nil {
		tv.logger.Errorf("error adding nodes: %v", err)
		return false
	}

	var tree *dag.Dag
	if abr.Height > 0 {
		tree = dag.NewDag(ctx, transPreviousTip, nodeStore)
	} else {
		tree = consensus.NewEmptyTree(ctx, string(abr.ObjectId), nodeStore)
	}

	chainTree, err := chaintree.NewChainTree(
		ctx,
		tree,
		tv.validators,
		tv.group.Config().Transactions,
	)
	if err != nil {
		tv.logger.Errorf("error creating chaintree (tip: %s, nodes: %d): %v", transPreviousTip.String(), len(cborNodes), err)
		return false
	}

	root := &chaintree.RootNode{}

	err = chainTree.Dag.ResolveInto(ctx, []string{}, root)
	if err != nil {
		tv.logger.Errorf("error decoding root: %v", err)
		return false
	}

	if root.Id != string(abr.ObjectId) {
		tv.logger.Warningf("abr did != chaintree did")
		return false
	}

	if (root.Height == 0 && abr.Height > 1) || (root.Height > 0 && abr.Height != root.Height+1) {
		tv.logger.Warningf("invalid height on ABR root: %d, abr: %d", root.Height, abr.Height)
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
		tv.logger.Errorf("error processing: %v", errMsg)
		return false
	}

	// TODO: Remove MagicValidNewTip check when done w/ gossip3 dark traffic testing!
	if string(abr.NewTip) != MagicValidNewTip {
		newTip, err := cid.Cast(abr.NewTip)
		if err != nil {
			tv.logger.Errorf("error casting abr new tip: %v", err)
			return false
		}

		if !chainTree.Dag.Tip.Equals(newTip) {
			sp.SetTag("tips-match", false)
			return false
		}
	}

	sp.SetTag("tips-match", true)

	return true
}

func pubsubMsgToAddBlockRequest(ctx context.Context, msg *pubsub.Message) (*services.AddBlockRequest, error) {
	sp, _ := opentracing.StartSpanFromContext(ctx, "gossip4.unmarshalPubSub")
	defer sp.Finish()

	abr := &services.AddBlockRequest{}
	err := abr.Unmarshal(msg.Data)
	return abr, err
}
