package gossip4

import (
	"context"
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-hamt-ipld"
	logging "github.com/ipfs/go-log"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/messages/build/go/services"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
)

var logger = logging.Logger("dagcoms")

const transactionTopic = "g4-transactions"

type Node struct {
	p2pNode     p2p.Node
	signKey     *bls.SignKey
	notaryGroup *types.NotaryGroup
	// pubsub    remote.PubSub
	dagStore  nodestore.DagStore
	kVStore   datastore.Batching
	hamtStore *hamt.CborIpldStore
	pubsub    *pubsub.PubSub

	latestCheckpoint     *Checkpoint
	inprogressCheckpoint *Checkpoint

	inFlight map[string]*services.AddBlockRequest
}

type NewNodeOptions struct {
	P2PNode          p2p.Node
	SignKey          *bls.SignKey
	NotaryGroup      *types.NotaryGroup
	DagStore         nodestore.DagStore
	KVStore          datastore.Batching
	latestCheckpoint cid.Cid
}

func NewNode(ctx context.Context, opts *NewNodeOptions) (*Node, error) {
	hamtStore := dagStoreToCborIpld(opts.DagStore)

	var latest *Checkpoint
	// if latestCheckpoint is undef then just make an empty hamt
	if opts.latestCheckpoint.Equals(cid.Undef) {
		n := hamt.NewNode(hamtStore, hamt.UseTreeBitWidth(5))

		latest = &Checkpoint{
			CurrentState: cid.Undef,
			Height:       0,
			Previous:     cid.Undef,
			node:         n,
		}
	} else {
		// otherwise pull up the CheckPoint
		err := hamtStore.Get(ctx, opts.latestCheckpoint, latest)
		if err != nil {
			return nil, fmt.Errorf("error getting latest: %v", err)
		}
	}

	// then clone for the inprogress
	inProgress := latest.Copy()

	return &Node{
		p2pNode:              opts.P2PNode,
		signKey:              opts.SignKey,
		notaryGroup:          opts.NotaryGroup,
		dagStore:             opts.DagStore,
		kVStore:              opts.KVStore,
		hamtStore:            hamtStore,
		latestCheckpoint:     latest,
		inprogressCheckpoint: inProgress,
		inFlight:             make(map[string]*services.AddBlockRequest),
	}, nil
}

func (n *Node) Start(ctx context.Context) error {
	pid := actor.EmptyRootContext.Spawn(actor.PropsFromFunc(n.Receive))
	go func() {
		<-ctx.Done()
		actor.EmptyRootContext.Poison(pid)
	}()

	validator := &transactionValidator{
		group: n.notaryGroup,
		node:  pid,
	}
	err := validator.setup()
	if err != nil {
		return fmt.Errorf("error setting up: %v", err)
	}
	n.pubsub = n.p2pNode.GetPubSub()
	n.pubsub.RegisterTopicValidator(transactionTopic, validator.validate)
	sub, err := n.pubsub.Subscribe(transactionTopic)
	if err != nil {
		return fmt.Errorf("error subscribing %v", err)
	}
	go func() {
		for {
			_, err := sub.Next(ctx)
			if err != nil {
				logger.Warningf("error getting sub message: %v", err)
				return
			}
		}
	}()
	return nil
}

func (n *Node) Receive(actorContext actor.Context) {
	switch msg := actorContext.Message().(type) {
	case *services.AddBlockRequest:
		logger.Debugf("handling message: %v", msg)
		//
	}
}

/**
when a transaction comes in make sure it's internally consistent and the height is higher than current
return true to the pubsub validator and pipe the tx to the node.

At the node do the normal thing where if it's the next height, then it can go into the hamt as accepted.

Put it in the transaction hamt, update the current state hamt. If the difficulty level has hit, publish out the CheckPoint

On receiving a CheckPoint...
	* confirm it meets the difficulty level
	* confirm it's a height higher than where we are at
	* confirm all transactions are valid
	* sign it
	* publish out the updated signature

*/
