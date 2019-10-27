package gossip4

import (
	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-hamt-ipld"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"
)

type Node struct {
	p2pNode     p2p.Node
	signKey     *bls.SignKey
	notaryGroup *types.NotaryGroup
	// pubsub    remote.PubSub
	dagStore  nodestore.DagStore
	kVStore   datastore.Batching
	hamtStore *hamt.CborIpldStore
}

type NewNodeOptions struct {
	P2PNode     p2p.Node
	SignKey     *bls.SignKey
	NotaryGroup *types.NotaryGroup
	DagStore    nodestore.DagStore
	KVStore     datastore.Batching
}

func NewNode(opts *NewNodeOptions) *Node {
	hamtStore := dagStoreToCborIpld(opts.DagStore)
	// pubsub := opts.P2PNode.GetPubSub()
	return &Node{
		p2pNode:     opts.P2PNode,
		signKey:     opts.SignKey,
		notaryGroup: opts.NotaryGroup,
		dagStore:    opts.DagStore,
		kVStore:     opts.KVStore,
		hamtStore:   hamtStore,
	}
}

func (n *Node) Receive(actorCtx actor.Context) {

}
