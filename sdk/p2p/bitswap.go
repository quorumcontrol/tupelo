package p2p

import (
	"context"

	bitswap "github.com/ipfs/go-bitswap"
	"github.com/ipfs/go-bitswap/network"
	blockservice "github.com/ipfs/go-blockservice"
	cbor "github.com/ipfs/go-ipld-cbor"
	merkledag "github.com/ipfs/go-merkledag"

	cid "github.com/ipfs/go-cid"
	ipld "github.com/ipfs/go-ipld-format"
	dag "github.com/ipfs/go-merkledag"

	blockstore "github.com/ipfs/go-ipfs-blockstore"
)

func init() {
	ipld.Register(cid.DagProtobuf, dag.DecodeProtobufBlock)
	ipld.Register(cid.Raw, dag.DecodeRawBlock)
	ipld.Register(cid.DagCBOR, cbor.DecodeBlock) // need to decode CBOR
}

// BitswapPeer is for exchanging ipld blocks on the network
// it can be used in any of the LibP2PHost networks, but
// can also be used directly on the IPFS network.
type BitswapPeer struct {
	ipld.DAGService

	bstore blockstore.Blockstore
}

// NewBitswapPeer creates a new block-swapping peer from an existing *LibP2PHost
// It is important that you bootstrap *after* creating this peer. There is a helper function:
// NewHostAndBitSwapPeer which can create both the host and this peer at the same time.
func NewBitswapPeer(ctx context.Context, host *LibP2PHost, bitswapOpts ...bitswap.Option) (*BitswapPeer, error) {
	bswapnet := network.NewFromIpfsHost(host.host, host.routing)
	bswap := bitswap.New(ctx, bswapnet, host.blockstore, bitswapOpts...)
	bserv := blockservice.New(host.blockstore, bswap)

	dags := merkledag.NewDAGService(bserv)
	return &BitswapPeer{
		DAGService: dags,
		bstore:     host.blockstore,
	}, nil
}

// Session returns a session-based NodeGetter.
func (bp *BitswapPeer) Session(ctx context.Context) ipld.NodeGetter {
	ng := merkledag.NewSession(ctx, bp.DAGService)
	if ng == bp.DAGService {
		log.Warning("DAGService does not support sessions")
	}
	return ng
}

// BlockStore offers access to the blockstore underlying the Peer's DAGService.
func (bp *BitswapPeer) BlockStore() blockstore.Blockstore {
	return bp.bstore
}

// HasBlock returns whether a given block is available locally. It is
// a shorthand for .Blockstore().Has().
func (bp *BitswapPeer) HasBlock(c cid.Cid) (bool, error) {
	return bp.BlockStore().Has(c)
}
