package p2p

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"io"
	"io/ioutil"
	gonet "net"
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
)

var log = logging.Logger("libp2play")

var ErrDialBackoff = swarm.ErrDialBackoff

type Host struct {
	host      *rhost.RoutedHost
	routing   *dht.IpfsDHT
	publicKey *ecdsa.PublicKey
	Reporter  metrics.Reporter
}

// GetRandomUnusedPort returns a random unused port
func GetRandomUnusedPort() int {
	listener, _ := gonet.Listen("tcp", ":0")
	defer listener.Close()
	return listener.Addr().(*gonet.TCPAddr).Port
}

func p2pPrivateFromEcdsaPrivate(key *ecdsa.PrivateKey) (libp2pcrypto.PrivKey, error) {
	return libp2pcrypto.UnmarshalSecp256k1PrivateKey(key.D.Bytes())
}

func p2pPublicKeyFromEcdsaPublic(key *ecdsa.PublicKey) libp2pcrypto.PubKey {
	return (*libp2pcrypto.Secp256k1PublicKey)(key)
}

func PeerFromEcdsaKey(publicKey *ecdsa.PublicKey) (peer.ID, error) {
	return peer.IDFromPublicKey(p2pPublicKeyFromEcdsaPublic(publicKey))
}

func NewRelayHost(ctx context.Context, privateKey *ecdsa.PrivateKey, port int) (*Host, error) {
	return newHost(ctx, privateKey, port, true)
}

func NewHost(ctx context.Context, privateKey *ecdsa.PrivateKey, port int) (*Host, error) {
	return newHost(ctx, privateKey, port, false)
}

func newHost(ctx context.Context, privateKey *ecdsa.PrivateKey, port int, useRelay bool) (*Host, error) {
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

	return &Host{
		host:      routedHost,
		routing:   dht,
		publicKey: &privateKey.PublicKey,
		Reporter:  reporter,
	}, nil
}

func (h *Host) P2PIdentity() string {
	return h.host.ID().String()
}

func (h *Host) Bootstrap(peers []string) (io.Closer, error) {
	bootstrapCfg := BootstrapConfigWithPeers(convertPeers(peers))
	return Bootstrap(h.host, h.routing, bootstrapCfg)
}

func (h *Host) SetStreamHandler(protocol protocol.ID, handler func(net.Stream)) {
	h.host.SetStreamHandler(protocol, handler)
}

func (h *Host) NewStream(ctx context.Context, publicKey *ecdsa.PublicKey, protocol protocol.ID) (net.Stream, error) {
	peerID, err := peer.IDFromPublicKey(p2pPublicKeyFromEcdsaPublic(publicKey))
	if err != nil {
		return nil, fmt.Errorf("Could not convert public key to peer id: %v", err)
	}

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

func (h *Host) Send(publicKey *ecdsa.PublicKey, protocol protocol.ID, payload []byte) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := h.NewStream(ctx, publicKey, protocol)
	if err != nil {
		return fmt.Errorf("Error opening new stream: %v", err)
	}

	n, err := stream.Write(payload)
	if err != nil {
		return fmt.Errorf("Error writing message: %v", err)
	}
	log.Debugf("%s wrote %d bytes", h.host.ID().Pretty(), n)
	stream.Close()

	return nil
}

func (h *Host) SendAndReceive(publicKey *ecdsa.PublicKey, protocol protocol.ID, payload []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	stream, err := h.NewStream(ctx, publicKey, protocol)
	defer stream.Close()

	n, err := stream.Write(payload)
	if err != nil {
		return nil, fmt.Errorf("Error writing message: %v", err)
	}
	log.Debugf("%s wrote %d bytes", h.host.ID().Pretty(), n)

	return ioutil.ReadAll(stream)
}

func (h *Host) Addresses() []ma.Multiaddr {
	hostAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ipfs/%s", h.host.ID().Pretty()))
	addrs := make([]ma.Multiaddr, 0)
	for _, addr := range h.host.Addrs() {
		addrs = append(addrs, addr.Encapsulate(hostAddr))
	}
	return addrs
}

func (h *Host) PeerID() (peer.ID, error) {
	return PeerIDFromPublicKey(h.publicKey)
}

func PeerIDFromPublicKey(publicKey *ecdsa.PublicKey) (peer.ID, error) {
	return peer.IDFromPublicKey(p2pPublicKeyFromEcdsaPublic(publicKey))
}
