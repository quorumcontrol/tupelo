package p2p

import (
	"context"
	"crypto/ecdsa"
	"io"
	"time"

	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/libp2p/go-libp2p-core/protocol"
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
	SetStreamHandler(protocol protocol.ID, handler network.StreamHandler)
	NewStream(ctx context.Context, publicKey *ecdsa.PublicKey, protocol protocol.ID) (network.Stream, error)
	NewStreamWithPeerID(ctx context.Context, peerID peer.ID, protocol protocol.ID) (network.Stream, error)
	Send(publicKey *ecdsa.PublicKey, protocol protocol.ID, payload []byte) error
	SendAndReceive(publicKey *ecdsa.PublicKey, protocol protocol.ID, payload []byte) ([]byte, error)
	GetPubSub() *pubsub.PubSub
}
