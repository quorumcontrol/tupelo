package types

import (
	"context"
	"fmt"
	"strings"

	"github.com/ipfs/go-cid"
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

// TODO: Make this detect loops and error?
func (gro *GraftedOwnership) resolveOwnersRecursively(ctx context.Context, graftedDag graftabledag.GraftableDag) (Addrs, error) {
	uncastAuths, _, err := graftedDag.GlobalResolve(ctx, strings.Split("tree/"+consensus.TreePathForAuthentications, "/"))
	if err != nil {
		return nil, err
	}

	var owners Addrs
	switch auths := uncastAuths.(type) {
	case []interface{}:
		for _, uncastAuth := range auths {
			switch auth := uncastAuth.(type) {
			case *dag.Dag:
				nextGDag, err := graftabledag.New(auth, gro.graftedDag.DagGetter())
				if err != nil {
					return nil, err
				}
				nextAddrs, err := gro.resolveOwnersRecursively(ctx, nextGDag)
				if err != nil {
					return nil, err
				}
				owners = append(owners, nextAddrs...)
			case string:
				owners = append(owners, auth)
			case []interface{}:
				for _, uncastAddr := range auth {
					if addr, ok := uncastAddr.(string); ok {
						owners = append(owners, addr)
					} else {
						return nil, fmt.Errorf("unknown auth type for %+v: %T", uncastAddr, uncastAddr)
					}
				}
			default:
				return nil, fmt.Errorf("unknown auth type for %+v: %T", auth, auth)
			}
		}
	case *dag.Dag:
		nextGDag, err := graftabledag.New(auths, gro.graftedDag.DagGetter())
		if err != nil {
			return nil, err
		}
		nextAddrs, err := gro.resolveOwnersRecursively(ctx, nextGDag)
		if err != nil {
			return nil, err
		}
		owners = append(owners, nextAddrs...)
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
	return gro.resolveOwnersRecursively(ctx, gro.graftedDag)
}

