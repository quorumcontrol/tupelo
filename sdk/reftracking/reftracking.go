package reftracking

import (
	"context"
	"fmt"

	"github.com/ipfs/go-cid"
	format "github.com/ipfs/go-ipld-format"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
)

type cidTracker map[cid.Cid]struct{}

func (ct cidTracker) toSlice() []cid.Cid {
	ids := make([]cid.Cid, len(ct))
	i := 0
	for k := range ct {
		ids[i] = k
		i++
	}
	return ids
}

/*
StoreWrapper keeps tracks of all the Gets and Adds in a DagStore.
We use this because we want to know what from the previous state needs to be sent up to a Tupelo Signer
So when we're playing transactions against a tree, we swap out its store for this one and keep track of all the Gets and Adds.
The reason for keeping track of Adds is that a second Tx in a block might reference some *new* nodes created by the first,
but there is no reason to send those up to Tupelo in the state of the Tx.
*/
type StoreWrapper struct {
	nodestore.DagStore
	touched cidTracker
	added   cidTracker
}

func (sw *StoreWrapper) Get(ctx context.Context, id cid.Cid) (format.Node, error) {
	n, err := sw.DagStore.Get(ctx, id)
	if err == nil && n != nil {
		if _, ok := sw.added[id]; !ok {
			sw.touched[id] = struct{}{}
		}
	}
	return n, err
}

func (sw *StoreWrapper) Add(ctx context.Context, n format.Node) error {
	err := sw.DagStore.Add(ctx, n)
	if err == nil {
		sw.added[n.Cid()] = struct{}{}
	}
	return err
}

func (sw *StoreWrapper) TouchedNodes(ctx context.Context) ([]format.Node, error) {
	return sw.cidToNodes(ctx, sw.touched.toSlice())
}

func (sw *StoreWrapper) NewNodes(ctx context.Context) ([]format.Node, error) {
	return sw.cidToNodes(ctx, sw.added.toSlice())
}

func (sw *StoreWrapper) cidToNodes(ctx context.Context, ids []cid.Cid) ([]format.Node, error) {
	nodes := make([]format.Node, len(ids))
	for i, id := range ids {
		n, err := sw.DagStore.Get(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("error getting node: %v", err)
		}
		nodes[i] = n
	}
	return nodes, nil
}

func WrapStoreForRefCounting(store nodestore.DagStore) *StoreWrapper {
	return &StoreWrapper{
		DagStore: store,
		touched:  make(cidTracker),
		added:    make(cidTracker),
	}
}

// Creating a new dag & chaintree with a tracked datastore to ensure that only
// the necessary nodes are sent to the signers for a processed block. Processing
// blocks on the tracked chaintree will keep track of which nodes are accessed.
func WrapTree(ctx context.Context, untrackedTree *chaintree.ChainTree) (*chaintree.ChainTree, *StoreWrapper, error) {
	originalStore := untrackedTree.Dag.Store
	tracker := WrapStoreForRefCounting(originalStore)
	trackedTree, err := chaintree.NewChainTree(ctx, dag.NewDag(ctx, untrackedTree.Dag.Tip, tracker), untrackedTree.BlockValidators, untrackedTree.Transactors)

	return trackedTree, tracker, err
}
