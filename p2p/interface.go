package p2p

import (
	"context"
	"crypto/ecdsa"
	"io"
	"time"

	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	peer "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-peer"
	protocol "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-protocol"
	ma "github.com/ipsn/go-ipfs/gxlibs/github.com/multiformats/go-multiaddr"
)

// Node is the interface any p2p host must implement to be used
// The interface is here so that other implementations (like in memory for tests)
// can be used to, for example, simulate large networks on single machines
type Node interface {
	Identity() string
	PublicKey() *ecdsa.PublicKey
	Addresses() []ma.Multiaddr
	Bootstrap(peers []string) (io.Closer, error)
	WaitForBootstrap(peerCount int, timeout time.Duration) error
	SetStreamHandler(protocol protocol.ID, handler net.StreamHandler)
	NewStream(ctx context.Context, publicKey *ecdsa.PublicKey, protocol protocol.ID) (net.Stream, error)
	NewStreamWithPeerID(ctx context.Context, peerID peer.ID, protocol protocol.ID) (net.Stream, error)
	Send(publicKey *ecdsa.PublicKey, protocol protocol.ID, payload []byte) error
	SendAndReceive(publicKey *ecdsa.PublicKey, protocol protocol.ID, payload []byte) ([]byte, error)
}
