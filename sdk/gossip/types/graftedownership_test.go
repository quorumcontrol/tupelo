package types

import (
	"context"
	"fmt"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/messages/v2/build/go/transactions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/graftabledag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
)

type TestDagGetter struct {
	chaintrees map[string]*chaintree.ChainTree
}

var _ graftabledag.DagGetter = (*TestDagGetter)(nil)

func (tdg *TestDagGetter) GetTip(_ context.Context, did string) (*cid.Cid, error) {
	if ct, ok := tdg.chaintrees[did]; ok {
		return &ct.Dag.Tip, nil
	}

	return nil, fmt.Errorf("no tip found for %s", did)
}

func (tdg *TestDagGetter) GetLatest(_ context.Context, did string) (*chaintree.ChainTree, error) {
	if ct, ok := tdg.chaintrees[did]; ok {
		return ct, nil
	}

	return nil, fmt.Errorf("no chaintree found for %s", did)
}

func newChaintreeWithNodes(t *testing.T, ctx context.Context, name string, treeNodes map[string]interface{}) *chaintree.ChainTree {
	sw := &safewrap.SafeWrap{}

	treeMap := map[string]interface{}{
		"hithere": "hothere",
	}

	for k, v := range treeNodes {
		treeMap[k] = v
	}

	tree := sw.WrapObject(treeMap)

	chain := sw.WrapObject(make(map[string]string))

	root := sw.WrapObject(map[string]interface{}{
		"chain": chain.Cid(),
		"tree":  tree.Cid(),
		"id":    "did:tupelo:"+name,
	})

	store := nodestore.MustMemoryStore(ctx)
	ctDag, err := dag.NewDagWithNodes(ctx, store, root, tree, chain)
	require.Nil(t, err)
	chainTree, err := chaintree.NewChainTree(
		ctx,
		ctDag,
		[]chaintree.BlockValidatorFunc{},
		map[transactions.Transaction_Type]chaintree.TransactorFunc{},
	)
	require.Nil(t, err)

	return chainTree
}

func newChaintree(t *testing.T, ctx context.Context, name string) *chaintree.ChainTree {
	return newChaintreeWithNodes(t, ctx, name, map[string]interface{}{})
}

func newChaintreeOwnedBy(t *testing.T, ctx context.Context, name string, owners []string) *chaintree.ChainTree {
	return newChaintreeWithNodes(t, ctx, name, map[string]interface{}{
		"_tupelo": map[string]interface{}{
			"authentications": owners,
		},
	})
}

func newDagGetter(t *testing.T, ctx context.Context, chaintrees... *chaintree.ChainTree) *TestDagGetter {
	dagGetter := &TestDagGetter{
		chaintrees: make(map[string]*chaintree.ChainTree),
	}

	for _, ct := range chaintrees {
		did, err := ct.Id(ctx)
		require.Nil(t, err)
		dagGetter.chaintrees[did] = ct
	}

	return dagGetter
}

func TestResolveOwnersOriginKey(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ct := newChaintree(t, ctx, "uno")

	dg := newDagGetter(t, ctx, ct)

	originDag := ct.Dag

	gro, err := NewGraftedOwnership(originDag, dg)
	require.Nil(t, err)

	owners, err := gro.ResolveOwners(ctx)
	require.Nil(t, err)

	assert.Equal(t, 1, len(owners))
	assert.Contains(t, owners, "uno")
}

func TestResolveOwnersOneChaintreeOwnsAnother(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ct1 := newChaintree(t, ctx, "uno")
	did, err := ct1.Id(ctx)
	require.Nil(t, err)
	ct2 := newChaintreeOwnedBy(t, ctx, "dos", []string{did})

	dg := newDagGetter(t, ctx, ct1, ct2)

	originDag := ct2.Dag

	gro, err := NewGraftedOwnership(originDag, dg)
	require.Nil(t, err)

	owners, err := gro.ResolveOwners(ctx)
	require.Nil(t, err)

	assert.Equal(t, 1, len(owners))
	assert.Contains(t, owners, "uno")
}

func TestResolveOwnersOneChaintreeOwnedByTwoPeers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dids := make([]string, 0)

	ct1 := newChaintree(t, ctx, "uno")
	did, err := ct1.Id(ctx)
	require.Nil(t, err)
	dids = append(dids, did)

	ct2 := newChaintree(t, ctx, "dos")
	did, err = ct2.Id(ctx)
	require.Nil(t, err)
	dids = append(dids, did)

	ct3 := newChaintreeOwnedBy(t, ctx, "tres", dids)

	dg := newDagGetter(t, ctx, ct1, ct2, ct3)

	originDag := ct3.Dag

	gro, err := NewGraftedOwnership(originDag, dg)
	require.Nil(t, err)

	owners, err := gro.ResolveOwners(ctx)
	require.Nil(t, err)

	assert.Equal(t, 2, len(owners))
	assert.Contains(t, owners, "uno")
	assert.Contains(t, owners, "dos")
}

func TestResolveOwnersOneChaintreeOwnedByTwoPeersAndAnAddr(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	dids := make([]string, 0)

	ct1 := newChaintree(t, ctx, "uno")
	did, err := ct1.Id(ctx)
	require.Nil(t, err)
	dids = append(dids, did)

	ct2 := newChaintree(t, ctx, "dos")
	did, err = ct2.Id(ctx)
	require.Nil(t, err)
	dids = append(dids, did)

	dids = append(dids, "addr")

	ct3 := newChaintreeOwnedBy(t, ctx, "tres", dids)

	dg := newDagGetter(t, ctx, ct1, ct2, ct3)

	originDag := ct3.Dag

	gro, err := NewGraftedOwnership(originDag, dg)
	require.Nil(t, err)

	owners, err := gro.ResolveOwners(ctx)
	require.Nil(t, err)

	assert.Equal(t, 3, len(owners))
	assert.Contains(t, owners, "uno")
	assert.Contains(t, owners, "dos")
	assert.Contains(t, owners, "addr")
}

func TestResolveOwnersOneChaintreeOwnedByParentAndGrandparent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ct1 := newChaintree(t, ctx, "uno")
	did, err := ct1.Id(ctx)
	require.Nil(t, err)

	ct2 := newChaintreeOwnedBy(t, ctx, "dos", []string{did, "otheraddr"})
	did, err = ct2.Id(ctx)
	require.Nil(t, err)

	ct3 := newChaintreeOwnedBy(t, ctx, "tres", []string{did})

	dg := newDagGetter(t, ctx, ct1, ct2, ct3)

	originDag := ct3.Dag

	gro, err := NewGraftedOwnership(originDag, dg)
	require.Nil(t, err)

	owners, err := gro.ResolveOwners(ctx)
	require.Nil(t, err)

	assert.Equal(t, 2, len(owners))
	assert.Contains(t, owners, "uno")
	assert.Contains(t, owners, "otheraddr")
}

func TestResolveOwnersArbitraryPath(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ct1 := newChaintreeWithNodes(t, ctx, "uno", map[string]interface{}{
		"data": map[string]interface{}{
			"otherChaintreeOwners": []string{"addr1", "addr2"},
		},
	})
	did, err := ct1.Id(ctx)
	require.Nil(t, err)

	ct2 := newChaintreeOwnedBy(t, ctx, "dos", []string{did+"/tree/data/otherChaintreeOwners", "otheraddr"})
	did, err = ct2.Id(ctx)
	require.Nil(t, err)

	ct3 := newChaintreeOwnedBy(t, ctx, "tres", []string{did})

	dg := newDagGetter(t, ctx, ct1, ct2, ct3)

	originDag := ct3.Dag

	gro, err := NewGraftedOwnership(originDag, dg)
	require.Nil(t, err)

	owners, err := gro.ResolveOwners(ctx)
	require.Nil(t, err)

	assert.Equal(t, 3, len(owners))
	assert.Contains(t, owners, "addr1")
	assert.Contains(t, owners, "addr2")
	assert.Contains(t, owners, "otheraddr")
}
