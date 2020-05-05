package types

import (
	"context"
	"fmt"
	"strings"

	"github.com/ipfs/go-cid"
	"github.com/ipfs/go-hamt-ipld"
	cbornode "github.com/ipfs/go-ipld-cbor"
	"github.com/quorumcontrol/messages/v2/build/go/gossip"
	"github.com/quorumcontrol/messages/v2/build/go/services"

	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/graftabledag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/tupelo/sdk/consensus"
)

type Addrs []string

type GraftedOwnershipResolver interface {
	ResolveOwners(ctx context.Context) (addrs Addrs, err error)
}

type GraftedOwnership struct {
	graftedDag graftabledag.GraftableDag
}

var _ GraftedOwnershipResolver = (*GraftedOwnership)(nil)

// client DagGetter

type GraftedOwnershipClient interface {
	GetTip(ctx context.Context, did string) (*gossip.Proof, error)
	GetLatest(ctx context.Context, did string) (*consensus.SignedChainTree, error)
}

type ClientDagGetter struct {
	groClient GraftedOwnershipClient
}

var _ graftabledag.DagGetter = (*ClientDagGetter)(nil)

func NewClientDagGetter(groc GraftedOwnershipClient) *ClientDagGetter {
	return &ClientDagGetter{
		groClient: groc,
	}
}

func (cdg *ClientDagGetter) GetTip(ctx context.Context, did string) (*cid.Cid, error) {
	proof, err := cdg.groClient.GetTip(ctx, did)
	if err != nil {
		return nil, err
	}

	tipBytes := proof.Tip
	tipCid, err := cid.Cast(tipBytes)
	if err != nil {
		return nil, err
	}

	return &tipCid, nil
}

func (cdg *ClientDagGetter) GetLatest(ctx context.Context, did string) (*chaintree.ChainTree, error) {
	sct, err := cdg.groClient.GetLatest(ctx, did)
	if err != nil {
		return nil, err
	}

	return sct.ChainTree, nil
}

// node DagGetter

type GraftedOwnershipNode interface {
	GetLastCompletedRound(ctx context.Context) (*RoundWrapper, error)
	GetAddBlockRequest(ctx context.Context, txCID *cid.Cid) (*services.AddBlockRequest, error)
	DagStore() nodestore.DagStore
	NotaryGroup() *NotaryGroup
}

type NodeDagGetter struct {
	groNode GraftedOwnershipNode
}

var _ graftabledag.DagGetter = (*NodeDagGetter)(nil)

func NewNodeDagGetter(gron GraftedOwnershipNode) *NodeDagGetter {
	return &NodeDagGetter{
		groNode: gron,
	}
}

func (ndg *NodeDagGetter) getLastABR(ctx context.Context, did string) (*services.AddBlockRequest, error) {
	rw, err := ndg.groNode.GetLastCompletedRound(ctx)
	if err != nil {
		return nil, err
	}

	hamtNode, err := rw.FetchHamt(ctx)
	if err != nil {
		return nil, err
	}

	txCID := &cid.Cid{}
	err = hamtNode.Find(ctx, did, txCID)
	if err != nil {
		if err == hamt.ErrNotFound {
			return nil, chaintree.ErrTipNotFound
		}
		return nil, err
	}

	abr, err := ndg.groNode.GetAddBlockRequest(ctx, txCID)
	if err != nil {
		return nil, err
	}

	return abr, nil
}

func (ndg *NodeDagGetter) GetTip(ctx context.Context, did string) (*cid.Cid, error) {
	abr, err := ndg.getLastABR(ctx, did)
	if err != nil {
		return nil, err
	}

	tipCid, err := cid.Cast(abr.NewTip)
	if err != nil {
		return nil, err
	}

	return &tipCid, nil
}

func (ndg *NodeDagGetter) GetLatest(ctx context.Context, did string) (*chaintree.ChainTree, error) {
	// TODO: Do we need to do all this?

	abr, err := ndg.getLastABR(ctx, did)
	if err != nil {
		return nil, err
	}

	newTip, err := cid.Cast(abr.NewTip)
	if err != nil {
		return nil, err
	}

	previousTip, err := cid.Cast(abr.PreviousTip)
	if err != nil {
		return nil, err
	}

	blockWithHeaders := &chaintree.BlockWithHeaders{}
	err = cbornode.DecodeInto(abr.Payload, blockWithHeaders)
	if err != nil {
		return nil, err
	}

	var treeDag *dag.Dag
	if abr.Height > 0 {
		treeDag = dag.NewDag(ctx, previousTip, ndg.groNode.DagStore())
	} else {
		treeDag = consensus.NewEmptyTree(ctx, string(abr.ObjectId), ndg.groNode.DagStore())
	}

	notaryGroup := ndg.groNode.NotaryGroup()
	blockValidators, err := notaryGroup.BlockValidators(ctx)
	if err != nil {
		return nil, err
	}
	transactorFuncs := notaryGroup.config.Transactions

	tree, err := chaintree.NewChainTree(ctx, treeDag, blockValidators, transactorFuncs)
	if err != nil {
		return nil, err
	}

	newTree, valid, err := tree.ProcessBlockImmutable(ctx, blockWithHeaders)
	if !valid || err != nil {
		return nil, fmt.Errorf("error processing block: %w", err)
	}

	if !newTree.Dag.Tip.Equals(newTip) {
		return nil, fmt.Errorf("error, tips did not match %s != %s", newTree.Dag.Tip.String(), newTip.String())
	}

	return newTree, nil
}

// GraftedOwnership

func NewGraftedOwnership(origin *dag.Dag, dagGetter graftabledag.DagGetter) (*GraftedOwnership, error) {
	graftableDag, err := graftabledag.New(origin, dagGetter)
	if err != nil {
		return nil, err
	}

	return &GraftedOwnership{
		graftedDag: graftableDag,
	}, nil
}

func (gro *GraftedOwnership) handleDag(ctx context.Context, d *dag.Dag, seen []chaintree.Path) (Addrs, error) {
	nextGDag, err := graftabledag.New(d, gro.graftedDag.DagGetter())
	if err != nil {
		return nil, fmt.Errorf("could not create graftable DAG for %+v: %w", d, err)
	}

	newOwners, err := gro.resolveOwnersRecursively(ctx, nextGDag, seen)
	if err != nil {
		return nil, err
	}

	return newOwners, nil
}

func (gro *GraftedOwnership) resolveOwnersRecursively(ctx context.Context, graftedDag graftabledag.GraftableDag, seen []chaintree.Path) (Addrs, error) {
	nextPath := strings.Split("tree/"+consensus.TreePathForAuthentications, "/")
	uncastDid, _, err := graftedDag.OriginDag().Resolve(ctx, []string{"id"})
	if err != nil {
		return nil, err
	}

	var (
		originDid string
		ok        bool
	)
	if originDid, ok = uncastDid.(string); !ok {
		return nil, fmt.Errorf("could not cast origin did to string; was a %T", uncastDid)
	}

	didPath := append([]string{originDid}, nextPath...)

	if graftabledag.PathsContainPrefix(seen, didPath) {
		return nil, fmt.Errorf("loop detected; some or all of %s has already been visited in this resolution", strings.Join(didPath, "/"))
	}

	seen = append(seen, didPath)

	uncastAuths, _, err := graftedDag.GlobalResolve(ctx, nextPath)
	if err != nil {
		return nil, err
	}

	var owners Addrs
	switch auths := uncastAuths.(type) {
	case []interface{}:
		for _, uncastAuth := range auths {
			switch auth := uncastAuth.(type) {
			case *dag.Dag:
				newOwners, err := gro.handleDag(ctx, auth, seen)
				if err != nil {
					return nil, err
				}
				owners = append(owners, newOwners...)
			case string:
				// filter out tupelo DIDs; means their chaintrees don't exist yet
				if !strings.HasPrefix(auth, "did:tupelo:") {
					owners = append(owners, auth)
				}
			case []interface{}:
				for _, uncastAddr := range auth {
					switch addr := uncastAddr.(type) {
					case string:
						owners = append(owners, addr)
					case *dag.Dag:
						newOwners, err := gro.handleDag(ctx, addr, seen)
						if err != nil {
							return nil, err
						}
						owners = append(owners, newOwners...)
					default:
						return nil, fmt.Errorf("unknown auth type for %+v: %T", uncastAddr, uncastAddr)
					}

				}
			default:
				return nil, fmt.Errorf("unknown auth type for %+v: %T", auth, auth)
			}
		}
	case *dag.Dag:
		newOwners, err := gro.handleDag(ctx, auths, seen)
		if err != nil {
			return nil, err
		}
		owners = append(owners, newOwners...)
	case nil:
		id, _, err := graftedDag.OriginDag().Resolve(ctx, []string{"id"})
		if err != nil {
			return nil, fmt.Errorf("could not get id: %w", err)
		}

		return Addrs{consensus.DidToAddr(id.(string))}, nil
	default:
		return nil, fmt.Errorf("got unexpected type for auths %+v: %T", auths, auths)
	}

	return owners, nil
}

func (gro *GraftedOwnership) ResolveOwners(ctx context.Context) (Addrs, error) {
	seen := make([]chaintree.Path, 0)
	return gro.resolveOwnersRecursively(ctx, gro.graftedDag, seen)
}
