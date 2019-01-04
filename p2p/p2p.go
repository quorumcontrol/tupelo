package p2p

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"io"
	"io/ioutil"
	"time"

	ds "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-datastore"
	dsync "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-datastore/sync"
	logging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	"github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p"
	circuit "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-circuit"
	libp2pcrypto "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-crypto"
	dht "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-kad-dht"
	metrics "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-metrics"
	net "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-net"
	peer "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-peer"
	protocol "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-protocol"
	swarm "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-swarm"
	rhost "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p/p2p/host/routed"
	ma "github.com/ipsn/go-ipfs/gxlibs/github.com/multiformats/go-multiaddr"
	opentracing "github.com/opentracing/opentracing-go"
)

var log = logging.Logger("libp2play")

var ErrDialBackoff = swarm.ErrDialBackoff

// Compile time assertion that Host implements Node
var _ Node = (*LibP2PHost)(nil)

type LibP2PHost struct {
	host            *rhost.RoutedHost
	routing         *dht.IpfsDHT
	publicKey       *ecdsa.PublicKey
	Reporter        metrics.Reporter
	bootstrapConfig *BootstrapConfig
}

const expectedKeySize = 32

func p2pPrivateFromEcdsaPrivate(key *ecdsa.PrivateKey) (libp2pcrypto.PrivKey, error) {
	// private keys can be 31 or 32 bytes for ecdsa.PrivateKey, but must be 32 Bytes for libp2pcrypto,
	// so we zero pad the slice if it is 31 bytes.
	keyBytes := key.D.Bytes()
	if (len(keyBytes) != expectedKeySize) && (len(keyBytes) != (expectedKeySize - 1)) {
		return nil, fmt.Errorf("error: length of private key must be 31 or 32 bytes")
	}
	keyBytes = append(make([]byte, expectedKeySize-len(keyBytes)), keyBytes...)
	libp2pKey, err := libp2pcrypto.UnmarshalSecp256k1PrivateKey(keyBytes)
	if err != nil {
		return libp2pKey, fmt.Errorf("error unmarshaling: %v", err)
	}
	return libp2pKey, err
}

func p2pPublicKeyFromEcdsaPublic(key *ecdsa.PublicKey) libp2pcrypto.PubKey {
	return (*libp2pcrypto.Secp256k1PublicKey)(key)
}

func PeerFromEcdsaKey(publicKey *ecdsa.PublicKey) (peer.ID, error) {
	return peer.IDFromPublicKey(p2pPublicKeyFromEcdsaPublic(publicKey))
}

func NewRelayLibP2PHost(ctx context.Context, privateKey *ecdsa.PrivateKey, port int) (*LibP2PHost, error) {
	return newLibP2PHost(ctx, privateKey, port, true)
}

func NewLibP2PHost(ctx context.Context, privateKey *ecdsa.PrivateKey, port int) (*LibP2PHost, error) {
	return newLibP2PHost(ctx, privateKey, port, false)
}

func newLibP2PHost(ctx context.Context, privateKey *ecdsa.PrivateKey, port int, useRelay bool) (*LibP2PHost, error) {
	go func() {
		<-ctx.Done()
		if span := opentracing.SpanFromContext(ctx); span != nil {
			span.Finish()
		}
	}()
	priv, err := p2pPrivateFromEcdsaPrivate(privateKey)
	if err != nil {
		return nil, err
	}
	// Generate a key pair for this host. We will use it at least
	// to obtain a valid host ID.
	reporter := metrics.NewBandwidthCounter()
	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port)),
		libp2p.Identity(priv),
		libp2p.DefaultTransports,
		libp2p.DefaultMuxers,
		libp2p.DefaultSecurity,
		libp2p.NATPortMap(),
		libp2p.BandwidthReporter(reporter),
	}

	if useRelay {
		opts = append(opts, libp2p.EnableRelay(circuit.OptActive, circuit.OptHop))
	}

	basicHost, err := libp2p.New(ctx, opts...)
	if err != nil {
		return nil, err
	}
	// Construct a datastore (needed by the DHT). This is just a simple, in-memory thread-safe datastore.
	dstore := dsync.MutexWrap(ds.NewMapDatastore())
	// Make the DHT
	dht := dht.NewDHT(ctx, basicHost, dstore)
	// Make the routed host
	routedHost := rhost.Wrap(basicHost, dht)

	return &LibP2PHost{
		host:      routedHost,
		routing:   dht,
		publicKey: &privateKey.PublicKey,
		Reporter:  reporter,
	}, nil
}

func (h *LibP2PHost) PublicKey() *ecdsa.PublicKey {
	return h.publicKey
}

func (h *LibP2PHost) Identity() string {
	return h.host.ID().Pretty()
}

func (h *LibP2PHost) Bootstrap(peers []string) (io.Closer, error) {
	bootstrapCfg := BootstrapConfigWithPeers(convertPeers(peers))
	h.bootstrapConfig = &bootstrapCfg
	return Bootstrap(h.host, h.routing, bootstrapCfg)
}

func (h *LibP2PHost) WaitForBootstrap(peerCount int, timeout time.Duration) error {
	if h.bootstrapConfig == nil {
		return fmt.Errorf("error must call Bootstrap() before calling WaitForBootstrap")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	doneCh := ctx.Done()
	defer cancel()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			connected := h.host.Network().Peers()
			if len(connected) >= peerCount {
				return nil
			}
		case <-doneCh:
			return fmt.Errorf("timeout waiting for bootstrap")
		}
	}
}

func (h *LibP2PHost) SetStreamHandler(protocol protocol.ID, handler net.StreamHandler) {
	h.host.SetStreamHandler(protocol, handler)
}

func (h *LibP2PHost) NewStream(ctx context.Context, publicKey *ecdsa.PublicKey, protocol protocol.ID) (net.Stream, error) {
	peerID, err := peer.IDFromPublicKey(p2pPublicKeyFromEcdsaPublic(publicKey))
	if err != nil {
		return nil, fmt.Errorf("Could not convert public key to peer id: %v", err)
	}
	return h.NewStreamWithPeerID(ctx, peerID, protocol)
}

func (h *LibP2PHost) NewStreamWithPeerID(ctx context.Context, peerID peer.ID, protocol protocol.ID) (net.Stream, error) {
	stream, err := h.host.NewStream(ctx, peerID, protocol)

	switch err {
	case swarm.ErrDialBackoff:
		return nil, ErrDialBackoff
	case nil:
		return stream, nil
	default:
		return stream, fmt.Errorf("Error opening stream: %v", err)
	}
}

func (h *LibP2PHost) Send(publicKey *ecdsa.PublicKey, protocol protocol.ID, payload []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := h.NewStream(ctx, publicKey, protocol)
	if err != nil {
		return fmt.Errorf("Error opening new stream: %v", err)
	}
	defer stream.Close()

	n, err := stream.Write(payload)
	if err != nil {
		return fmt.Errorf("Error writing message: %v", err)
	}
	log.Debugf("%s wrote %d bytes", h.host.ID().Pretty(), n)

	return nil
}

func (h *LibP2PHost) SendAndReceive(publicKey *ecdsa.PublicKey, protocol protocol.ID, payload []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := h.NewStream(ctx, publicKey, protocol)
	if err != nil {
		return nil, fmt.Errorf("error creating new stream")
	}
	defer stream.Close()

	n, err := stream.Write(payload)
	if err != nil {
		return nil, fmt.Errorf("Error writing message: %v", err)
	}
	log.Debugf("%s wrote %d bytes", h.host.ID().Pretty(), n)

	return ioutil.ReadAll(stream)
}

func (h *LibP2PHost) Addresses() []ma.Multiaddr {
	hostAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ipfs/%s", h.host.ID().Pretty()))
	addrs := make([]ma.Multiaddr, 0)
	for _, addr := range h.host.Addrs() {
		addrs = append(addrs, addr.Encapsulate(hostAddr))
	}
	return addrs
}

func PeerIDFromPublicKey(publicKey *ecdsa.PublicKey) (peer.ID, error) {
	return peer.IDFromPublicKey(p2pPublicKeyFromEcdsaPublic(publicKey))
}
