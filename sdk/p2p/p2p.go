package p2p

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"io"
	"io/ioutil"
	"sync"
	"time"

	ds "github.com/ipfs/go-datastore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p"
	libp2pcrypto "github.com/libp2p/go-libp2p-core/crypto"
	host "github.com/libp2p/go-libp2p-core/host"
	metrics "github.com/libp2p/go-libp2p-core/metrics"
	"github.com/libp2p/go-libp2p-core/network"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/protocol"
	"github.com/libp2p/go-libp2p-core/routing"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	dhtopts "github.com/libp2p/go-libp2p-kad-dht/opts"
	pnet "github.com/libp2p/go-libp2p-pnet"
	pubsub "github.com/libp2p/go-libp2p-pubsub"
	swarm "github.com/libp2p/go-libp2p-swarm"
	basichost "github.com/libp2p/go-libp2p/p2p/host/basic"
	rhost "github.com/libp2p/go-libp2p/p2p/host/routed"
	ma "github.com/multiformats/go-multiaddr"
	opentracing "github.com/opentracing/opentracing-go"
	"github.com/quorumcontrol/chaintree/cachedblockstore"
	"golang.org/x/xerrors"
)

var log = logging.Logger("tupelop2p")

var ErrDialBackoff = swarm.ErrDialBackoff

// Compile time assertion that Host implements Node
var _ Node = (*LibP2PHost)(nil)

type LibP2PHost struct {
	Reporter metrics.Reporter

	host             *rhost.RoutedHost
	routing          *dht.IpfsDHT
	publicKey        *ecdsa.PublicKey
	bootstrapStarted bool
	pubsub           *pubsub.PubSub
	datastore        ds.Batching
	blockstore       blockstore.Blockstore
	parentCtx        context.Context
	discoverers      map[string]*tupeloDiscoverer
	discoverLock     *sync.Mutex
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

func EcdsaKeyFromPeer(pid peer.ID) (*ecdsa.PublicKey, error) {
	pubkey, err := pid.ExtractPublicKey()
	if err != nil {
		return nil, fmt.Errorf("error extracting pubkey from peer: %v", err)
	}

	asSecp256k1PublicKey, ok := pubkey.(*libp2pcrypto.Secp256k1PublicKey)
	if !ok {
		return nil, fmt.Errorf("unsupported key type %s from peer id, must be Secp256k1", pubkey.Type().String())
	}

	return (*ecdsa.PublicKey)(asSecp256k1PublicKey), nil
}

func NewHostAndBitSwapPeer(ctx context.Context, userOpts ...Option) (*LibP2PHost, *BitswapPeer, error) {
	c := &Config{}
	opts := append(defaultOptions(), userOpts...)
	err := applyOptions(c, opts...)
	if err != nil {
		return nil, nil, fmt.Errorf("error applying opts: %v", opts)
	}

	h, err := newLibP2PHostFromConfig(ctx, c)
	if err != nil {
		return nil, nil, fmt.Errorf("error generating libp2p host: %v", err)
	}

	peer, err := NewBitswapPeer(ctx, h, c.BitswapOptions...)
	if err != nil {
		return nil, nil, fmt.Errorf("error generating bitswap peer: %v", err)
	}
	return h, peer, nil
}

func NewHostFromOptions(ctx context.Context, userOpts ...Option) (*LibP2PHost, error) {
	c := &Config{}
	opts := append(defaultOptions(), userOpts...)
	err := applyOptions(c, opts...)
	if err != nil {
		return nil, fmt.Errorf("error applying opts: %v", err)
	}
	return newLibP2PHostFromConfig(ctx, c)
}

func NewRelayLibP2PHost(ctx context.Context, privateKey *ecdsa.PrivateKey, port int) (*LibP2PHost, error) {
	log.Debug("constructing new relay libp2p host")
	cfg, err := backwardsCompatibleConfig(privateKey, port, true)
	if err != nil {
		return nil, fmt.Errorf("error generating config: %v", err)
	}
	return newLibP2PHostFromConfig(ctx, cfg)
}

func NewLibP2PHost(ctx context.Context, privateKey *ecdsa.PrivateKey, port int) (*LibP2PHost, error) {
	log.Debug("constructing new libp2p host")
	cfg, err := backwardsCompatibleConfig(privateKey, port, false)
	if err != nil {
		return nil, fmt.Errorf("error generating config: %v", err)
	}
	return newLibP2PHostFromConfig(ctx, cfg)
}

func newLibP2PHostFromConfig(ctx context.Context, c *Config) (*LibP2PHost, error) {
	go func() {
		<-ctx.Done()
		if span := opentracing.SpanFromContext(ctx); span != nil {
			span.Finish()
		}
	}()

	priv, err := p2pPrivateFromEcdsaPrivate(c.PrivateKey)
	if err != nil {
		return nil, err
	}

	var idht *dht.IpfsDHT

	if c.EnableWebsocket {
		ip := c.ListenIP
		if ip == "" {
			ip = "0.0.0.0"
		}
		c.ListenAddrs = append(c.ListenAddrs, fmt.Sprintf("/ip4/%s/tcp/%d/ws", ip, c.WebsocketPort))
	}

	var transports libp2p.Option
	if len(c.Transports) == 0 {
		transports = libp2p.DefaultTransports
	} else {
		transports = libp2p.ChainOptions(c.Transports...)
	}

	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(c.ListenAddrs...),
		libp2p.Identity(priv),
		transports,
		libp2p.DefaultMuxers,
		libp2p.DefaultSecurity,
		libp2p.BandwidthReporter(c.BandwidthReporter),
		libp2p.Routing(func(h host.Host) (routing.PeerRouting, error) {
			// make the DHT with the given Host
			opts := []dhtopts.Option{
				dhtopts.Datastore(c.DataStore),
			}
			if c.ClientOnlyDHT {
				opts = append(opts, dhtopts.Client(true))
			}
			rting, err := dht.New(ctx, h, opts...)
			if err == nil {
				idht = rting
			}
			return rting, err
		}),
	}

	if c.EnableAutoRelay {
		opts = append(opts, libp2p.EnableAutoRelay())
	}

	if len(c.AddrFilters) > 0 {
		opts = append(opts, libp2p.FilterAddresses(c.AddrFilters...))
	}

	if len(c.ExternalAddrs) > 0 {
		extmAddrs := make([]ma.Multiaddr, len(c.ExternalAddrs))
		for k, addr := range c.ExternalAddrs {
			mAddr, err := ma.NewMultiaddr(addr)
			if err != nil {
				return nil, fmt.Errorf("Error creating Multiaddr for ExternalAddrs: %v", err)
			}
			extmAddrs[k] = mAddr
		}

		opts = append(opts, libp2p.AddrsFactory(basichost.AddrsFactory(func(addrs []ma.Multiaddr) []ma.Multiaddr {
			return append(addrs, extmAddrs...)
		})))
	}

	if len(c.RelayOpts) > 0 {
		opts = append(opts, libp2p.EnableRelay(c.RelayOpts...))
	}

	// Create protector if we have a secret.
	if len(c.Segmenter) > 0 {
		var key [32]byte
		copy(key[:], c.Segmenter)
		prot, err := pnet.NewV1ProtectorFromBytes(&key)
		if err != nil {
			return nil, fmt.Errorf("error creating protected network: %v", err)
		}
		opts = append(opts, libp2p.PrivateNetwork(prot))
	}

	opts = append(opts, c.AdditionalP2POptions...)

	basicHost, err := libp2p.New(ctx, opts...)
	if err != nil {
		return nil, err
	}

	routedHost := basicHost.(*rhost.RoutedHost) // because of the routing option above, we got a routed host and not a basic one.

	var pubCreator func(context.Context, host.Host, ...pubsub.Option) (*pubsub.PubSub, error)
	switch c.PubSubRouter {
	case "gossip":
		pubCreator = pubsub.NewGossipSub
	case "floodsub":
		pubCreator = pubsub.NewFloodSub
	case "random":
		pubCreator = pubsub.NewRandomSub
	}

	pub, err := pubCreator(ctx, routedHost, c.PubSubOptions...)
	if err != nil {
		return nil, fmt.Errorf("error creating new gossip sub: %v", err)
	}

	var hostBlockStore blockstore.Blockstore

	if c.Blockstore == nil {
		// create a default blockstore
		bs := blockstore.NewBlockstore(c.DataStore)
		bs = blockstore.NewIdStore(bs)
		wrapped, err := cachedblockstore.WrapInCache(bs, 100)
		if err != nil {
			return nil, xerrors.Errorf("error wrapping: %w", err)
		}
		hostBlockStore = wrapped
	} else {
		hostBlockStore = c.Blockstore
	}

	h := &LibP2PHost{
		host:         routedHost,
		routing:      idht,
		publicKey:    &c.PrivateKey.PublicKey,
		Reporter:     c.BandwidthReporter,
		pubsub:       pub,
		parentCtx:    ctx,
		datastore:    c.DataStore,
		blockstore:   hostBlockStore,
		discoverLock: new(sync.Mutex),
		discoverers:  make(map[string]*tupeloDiscoverer),
	}

	if len(c.DiscoveryNamespaces) > 0 {
		for _, namespace := range c.DiscoveryNamespaces {
			h.discoverers[namespace] = newTupeloDiscoverer(h, namespace)
		}
	}

	return h, nil

}

func (h *LibP2PHost) PublicKey() *ecdsa.PublicKey {
	return h.publicKey
}

func (h *LibP2PHost) Identity() string {
	return h.host.ID().Pretty()
}

func (h *LibP2PHost) GetPubSub() *pubsub.PubSub {
	return h.pubsub
}

func (h *LibP2PHost) Bootstrap(peers []string) (io.Closer, error) {
	minPeers := 4
	if len(peers) < minPeers {
		minPeers = len(peers)
	}
	bootstrapper := NewBootstrapper(convertPeers(peers), h.host, h.host.Network(), h.routing,
		minPeers, 2*time.Second)
	bootstrapper.Start(h.parentCtx)
	h.bootstrapStarted = true

	for _, discoverer := range h.discoverers {
		err := discoverer.start(h.parentCtx)
		if err != nil {
			return nil, fmt.Errorf("error starting discovery for %s: %v", discoverer.namespace, err)
		}
	}

	return bootstrapper, nil
}

func (h *LibP2PHost) StartDiscovery(namespace string) error {
	h.discoverLock.Lock()

	discoverer, ok := h.discoverers[namespace]
	if !ok {
		discoverer = newTupeloDiscoverer(h, namespace)
		h.discoverers[namespace] = discoverer
	}
	h.discoverLock.Unlock()
	return discoverer.start(h.parentCtx)
}

func (h *LibP2PHost) StopDiscovery(namespace string) {
	h.discoverLock.Lock()
	defer h.discoverLock.Unlock()

	discoverer, ok := h.discoverers[namespace]
	if ok {
		discoverer.stop()
	}
	delete(h.discoverers, namespace)
}

func (h *LibP2PHost) WaitForBootstrap(peerCount int, timeout time.Duration) error {
	if !h.bootstrapStarted {
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
			log.Debugf("connected: %d", len(connected))
			if len(connected) >= peerCount {
				return nil
			}
		case <-doneCh:
			return fmt.Errorf("timeout waiting for bootstrap")
		}
	}
}

func (h *LibP2PHost) WaitForDiscovery(namespace string, num int, duration time.Duration) error {
	discoverer, ok := h.discoverers[namespace]
	if !ok {
		return fmt.Errorf("error missing discoverer (%s)", namespace)
	}

	return discoverer.waitForNumber(num, duration)
}

func (h *LibP2PHost) SetStreamHandler(protocol protocol.ID, handler network.StreamHandler) {
	h.host.SetStreamHandler(protocol, handler)
}

func (h *LibP2PHost) NewStream(ctx context.Context, publicKey *ecdsa.PublicKey, protocol protocol.ID) (network.Stream, error) {
	peerID, err := peer.IDFromPublicKey(p2pPublicKeyFromEcdsaPublic(publicKey))
	if err != nil {
		return nil, fmt.Errorf("Could not convert public key to peer id: %v", err)
	}
	return h.NewStreamWithPeerID(ctx, peerID, protocol)
}

func (h *LibP2PHost) NewStreamWithPeerID(ctx context.Context, peerID peer.ID, protocol protocol.ID) (network.Stream, error) {
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

// Connect ensures there is a connection between this host and the peer with
// given public key. This is useful for bitswap exchanges where peers are known,
// allowing blocks to be exchanged directly, skipping the expensive DHT provides.
//
// Connections are generally persisted, use Disconnect to clean up unused connections.
func (h *LibP2PHost) Connect(ctx context.Context, publicKey *ecdsa.PublicKey) error {
	peerID, err := peer.IDFromPublicKey(p2pPublicKeyFromEcdsaPublic(publicKey))
	if err != nil {
		return fmt.Errorf("Could not convert public key to peer id: %v", err)
	}

	return h.host.Connect(ctx, peer.AddrInfo{ID: peerID})
}

// Disconnect closes all connections to the given peer.
func (h *LibP2PHost) Disconnect(ctx context.Context, publicKey *ecdsa.PublicKey) error {
	peerID, err := peer.IDFromPublicKey(p2pPublicKeyFromEcdsaPublic(publicKey))
	if err != nil {
		return fmt.Errorf("Could not convert public key to peer id: %v", err)
	}

	return h.host.Network().ClosePeer(peerID)
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

	n, err := stream.Write(payload)
	// Close for writing so that the remote knows there's nothing more to read
	// The remote can still write back to us though
	stream.Close()
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

func (h *LibP2PHost) Subscribe(topic string, opts ...pubsub.SubOpt) (*pubsub.Subscription, error) {
	return h.pubsub.Subscribe(topic, opts...)
}

func (h *LibP2PHost) Publish(topic string, data []byte) error {
	return h.pubsub.Publish(topic, data)
}

func PeerIDFromPublicKey(publicKey *ecdsa.PublicKey) (peer.ID, error) {
	return peer.IDFromPublicKey(p2pPublicKeyFromEcdsaPublic(publicKey))
}
