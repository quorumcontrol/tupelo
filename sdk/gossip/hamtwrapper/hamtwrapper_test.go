package hamtwrapper

import (
	"context"
	"testing"

	"github.com/ipfs/go-hamt-ipld"
	"github.com/stretchr/testify/require"

	"github.com/quorumcontrol/chaintree/nodestore"
)

func TestFlushSimilarity(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := nodestore.MustMemoryStore(ctx)
	cborStore := DagStoreToCborIpld(store)

	hamtNode := hamt.NewNode(cborStore)

	err := hamtNode.Set(ctx, "test", 1)
	require.Nil(t, err)
	err = hamtNode.Set(ctx, "test2", 2)
	require.Nil(t, err)

	id, err := cborStore.Put(ctx, hamtNode)
	require.Nil(t, err)

	err = hamtNode.Flush(ctx)
	require.Nil(t, err)

	idAfterFlush, err := cborStore.Put(ctx, hamtNode)
	require.Nil(t, err)

	require.True(t, id.Equals(idAfterFlush))
}
