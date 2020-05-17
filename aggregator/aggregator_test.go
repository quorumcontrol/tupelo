package aggregator

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo/sdk/gossip/testhelpers"
	"github.com/quorumcontrol/tupelo/sdk/gossip/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewAggregator(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ng := types.NewNotaryGroup("testnotary")

	_, err := NewAggregator(ctx, NewMemoryStore(), ng)
	require.Nil(t, err)
}

func TestAddingAbrs(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ng := types.NewNotaryGroup("testnotary")

	agg, err := NewAggregator(ctx, NewMemoryStore(), ng)
	require.Nil(t, err)

	t.Run("new ABR, no existing works", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		abr := testhelpers.NewValidTransaction(t)

		_, err := agg.Add(ctx, &abr)
		require.Nil(t, err)
	})

	t.Run("conflicting ABR errors", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		treeKey, err := crypto.GenerateKey()
		require.Nil(t, err)

		abr1 := testhelpers.NewValidTransactionWithPathAndValue(t, treeKey, "/path", "value")
		abr2 := testhelpers.NewValidTransactionWithPathAndValue(t, treeKey, "/path", "differentvalue")

		_, err = agg.Add(ctx, &abr1)
		require.Nil(t, err)
		_, err = agg.Add(ctx, &abr2)
		require.NotNil(t, err)
	})
}

func TestGetLatest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ng := types.NewNotaryGroup("testnotary")

	agg, err := NewAggregator(ctx, NewMemoryStore(), ng)
	require.Nil(t, err)

	t.Run("saves state", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		treeKey, err := crypto.GenerateKey()
		require.Nil(t, err)

		abr1 := testhelpers.NewValidTransactionWithPathAndValue(t, treeKey, "/path", "value")
		_, err = agg.Add(ctx, &abr1)
		require.Nil(t, err)

		tree, err := agg.GetLatest(ctx, string(abr1.ObjectId))
		require.Nil(t, err)

		resp, remain, err := tree.Dag.Resolve(ctx, []string{"tree", "data", "path"})
		require.Nil(t, err)
		assert.Len(t, remain, 0)
		assert.Equal(t, "value", resp)
	})
}

// BenchmarkAdd-12    	    1504	    861040 ns/op	  199471 B/op	    2992 allocs/op
func BenchmarkAdd(b *testing.B) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ng := types.NewNotaryGroup("testnotary")

	agg, err := NewAggregator(ctx, NewMemoryStore(), ng)
	require.Nil(b, err)

	txs := make([]*services.AddBlockRequest, b.N)
	for i := 0; i < b.N; i++ {
		abr := testhelpers.NewValidTransaction(b)
		txs[i] = &abr
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err = agg.Add(ctx, txs[i])
	}
	b.StopTimer()
	require.Nil(b, err)
}
