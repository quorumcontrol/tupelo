package p2p

import (
	"context"
	"testing"
	"time"

	bitswap "github.com/ipfs/go-bitswap"
	"github.com/quorumcontrol/chaintree/safewrap"

	"github.com/stretchr/testify/require"
)

func twoBitswappingNodes(t *testing.T, ctx context.Context, opts ...Option) (nodeA *LibP2PHost, peerA *BitswapPeer, nodeB *LibP2PHost, peerB *BitswapPeer) {
	nodeA, peerA, err := NewHostAndBitSwapPeer(ctx)
	require.Nil(t, err)

	nodeB, peerB, err = NewHostAndBitSwapPeer(ctx)
	require.Nil(t, err)

	// Notice that the bootstrap is below the creation of the peer
	// THIS IS IMPORTANT

	_, err = nodeA.Bootstrap(bootstrapAddresses(nodeB))
	require.Nil(t, err)

	err = nodeA.WaitForBootstrap(1, 1*time.Second)
	require.Nil(t, err)

	_, err = nodeB.Bootstrap(bootstrapAddresses(nodeA))
	require.Nil(t, err)

	err = nodeB.WaitForBootstrap(1, 1*time.Second)
	require.Nil(t, err)

	return // nodea,peera,nodeb,peerb
}

func TestBitSwap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, peerA, _, peerB := twoBitswappingNodes(t, ctx)

	sw := &safewrap.SafeWrap{}

	n := sw.WrapObject(map[string]string{"hello": "bitswap"})
	require.Nil(t, sw.Err)

	swapCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := peerA.Add(swapCtx, n)
	require.Nil(t, err)

	_, err = peerB.Get(swapCtx, n.Cid())
	if err != nil {
		t.Error(err)
	}
}

func TestBitswapNoProvide(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, peerA, _, peerB := twoBitswappingNodes(t, ctx, WithBitswapOptions(bitswap.ProvideEnabled(false)))

	sw := &safewrap.SafeWrap{}

	n := sw.WrapObject(map[string]string{"hello": "bitswap"})
	require.Nil(t, sw.Err)

	swapCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	err := peerA.Add(swapCtx, n)
	require.Nil(t, err)

	_, err = peerB.Get(swapCtx, n.Cid())
	if err != nil {
		t.Error(err)
	}
}
