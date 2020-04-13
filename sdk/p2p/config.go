package p2p

import (
	"crypto/ecdsa"
	"fmt"
	"net"
	"os"

	bitswap "github.com/ipfs/go-bitswap"

	"github.com/ethereum/go-ethereum/crypto"
	ds "github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	circuit "github.com/libp2p/go-libp2p-circuit"
	"github.com/libp2p/go-libp2p-core/metrics"
	pubsub "github.com/libp2p/go-libp2p-pubsub"

	"github.com/libp2p/go-libp2p"
	ma "github.com/multiformats/go-multiaddr"
)

// Option is a configuration option for the server
type Option func(c *Config) error

type Config struct {
	RelayOpts            []circuit.RelayOpt
	EnableRelayHop       bool
	EnableAutoRelay      bool
	EnableWebsocket      bool
	WebsocketPort        int
	PubSubRouter         string
	PubSubOptions        []pubsub.Option
	PrivateKey           *ecdsa.PrivateKey
	EnableNATMap         bool
	ExternalAddrs        []string
	ListenAddrs          []string
	AddrFilters          []*net.IPNet
	Port                 int
	ListenIP             string
	DiscoveryNamespaces  []string
	AdditionalP2POptions []libp2p.Option
	Transports           []libp2p.Option // these should only be libp2p.Transport options, but can't detect that with type
	DataStore            ds.Batching
	Blockstore           blockstore.Blockstore
	BandwidthReporter    metrics.Reporter
	Segmenter            []byte
	ClientOnlyDHT        bool
	BitswapOptions       []bitswap.Option
}

// This is a function, because we want to return a new datastore each time
func defaultOptions() []Option {
	key, err := crypto.GenerateKey()
	if err != nil {
		panic(fmt.Errorf("unable to generate a new key"))
	}
	return []Option{
		WithKey(key),
		WithPubSubRouter("gossip"),
		WithPubSubOptions(pubsub.WithStrictSignatureVerification(false), pubsub.WithMessageSigning(false)),
		WithNATMap(),
		WithListenIP("0.0.0.0", 0),
		WithBandwidthReporter(metrics.NewBandwidthCounter()),
		WithDatastore(dsync.MutexWrap(ds.NewMapDatastore())),
	}
}

func applyOptions(c *Config, opts ...Option) error {
	for _, factory := range opts {
		err := factory(c)
		if err != nil {
			return fmt.Errorf("error applying option: %v", err)
		}
	}
	return nil
}

func backwardsCompatibleConfig(key *ecdsa.PrivateKey, port int, useRelay bool) (*Config, error) {
	c := &Config{}
	opts := defaultOptions()

	backwardsOpts := []Option{
		WithKey(key),
		WithDiscoveryNamespaces("tupelo-transaction-gossipers"),
		WithListenIP("0.0.0.0", port),
	}
	opts = append(opts, backwardsOpts...)

	if hostIP, ok := os.LookupEnv("TUPELO_PUBLIC_IP"); ok {
		log.Debugf("configuring libp2p external IP %s", hostIP)
		opts = append(opts, WithExternalIP(hostIP, port))
	}

	if useRelay {
		opts = append(opts, WithRelayOpts(circuit.OptActive, circuit.OptHop))
	}

	err := applyOptions(c, opts...)
	if err != nil {
		return nil, fmt.Errorf("error applying opts: %v", err)
	}

	return c, nil
}

// WithAddrFilters takes a string of cidr addresses (0.0.0.0/32) that will not
// be dialed by swarm.
func WithAddrFilters(addrFilters []string) Option {
	return func(c *Config) error {
		addrFilterIPs := make([]*net.IPNet, len(addrFilters))
		for i, cidr := range addrFilters {
			net, err := stringToIPNet(cidr)
			if err != nil {
				return fmt.Errorf("error getting stringToIPnet: %v", err)
			}
			addrFilterIPs[i] = net
		}
		c.AddrFilters = addrFilterIPs
		return nil
	}
}

// WithAutoRelay enabled AutoRelay in the config, defaults to false
func WithAutoRelay(enabled bool) Option {
	return func(c *Config) error {
		c.EnableAutoRelay = enabled
		return nil
	}
}

// WithDiscoveryNamespaces enables discovery of all of the passed in namespaces
// discovery is part of bootstrap, defaults to empty
func WithDiscoveryNamespaces(namespaces ...string) Option {
	return func(c *Config) error {
		c.DiscoveryNamespaces = namespaces
		return nil
	}
}

// WithSegmenter enables the secret on libp2p in order to make sure
// that this network does not combine with another. Default is off.
func WithSegmenter(secret []byte) Option {
	return func(c *Config) error {
		c.Segmenter = secret
		return nil
	}
}

// WithBandwidthReporter sets the bandwidth reporter,
// defaults to a new metrics.Reporter
func WithBandwidthReporter(reporter metrics.Reporter) Option {
	return func(c *Config) error {
		c.BandwidthReporter = reporter
		return nil
	}
}

// WithPubSubOptions sets pubsub options.
// Defaults to pubsub.WithStrictSignatureVerification(false), pubsub.WithMessageSigning(false)
func WithPubSubOptions(opts ...pubsub.Option) Option {
	return func(c *Config) error {
		c.PubSubOptions = opts
		return nil
	}
}

// WithDatastore sets the datastore used by the host
// defaults to in-memory map store.
func WithDatastore(store ds.Batching) Option {
	return func(c *Config) error {
		c.DataStore = store
		return nil
	}
}

// WithBlockStore sets the datastore used by the host
// defaults to nil which will wrap the underlying
// node datastore.
func WithBlockstore(store blockstore.Blockstore) Option {
	return func(c *Config) error {
		c.Blockstore = store
		return nil
	}
}

// WithRelayOpts turns on relay and sets the options
// defaults to empty (and relay off).
func WithRelayOpts(opts ...circuit.RelayOpt) Option {
	return func(c *Config) error {
		c.RelayOpts = opts
		return nil
	}
}

// WithKey is the identity key of the host, if not set it
// the default options will generate you a new key
func WithKey(key *ecdsa.PrivateKey) Option {
	return func(c *Config) error {
		c.PrivateKey = key
		return nil
	}
}

// WithListenIP sets the listen IP, defaults to 0.0.0.0/0
func WithListenIP(ip string, port int) Option {
	return func(c *Config) error {
		c.Port = port
		c.ListenIP = ip
		c.ListenAddrs = []string{fmt.Sprintf("/ip4/%s/tcp/%d", ip, c.Port)}
		return nil
	}
}

// WithWebSockets turns websockets on at specified port (default to 0)
func WithWebSockets(port int) Option {
	return func(c *Config) error {
		c.EnableWebsocket = true
		c.WebsocketPort = port
		return nil
	}
}

var supportedGossipTypes = map[string]bool{"gossip": true, "random": true, "floodsub": true}

// WithPubSubRouter sets the router type of the pubsub. Supported is: gossip, random, floodsub
// defaults to gossip
func WithPubSubRouter(routeType string) Option {
	return func(c *Config) error {
		if _, ok := supportedGossipTypes[routeType]; !ok {
			return fmt.Errorf("%s is an unsupported gossip type", routeType)
		}
		c.PubSubRouter = routeType
		return nil
	}
}

// WithNATMap enables nat mapping
func WithNATMap() Option {
	return func(c *Config) error {
		c.EnableNATMap = true
		return nil
	}
}

// WithExternalAddr sets an arbitrary multiaddr formatted string for broadcasting to swarm
func WithExternalAddr(addr string) Option {
	return func(c *Config) error {
		// Just ensure address can be casted to multiaddr successfully
		_, err := ma.NewMultiaddr(addr)
		if err != nil {
			return fmt.Errorf("Error creating multiaddress: %v", err)
		}
		c.ExternalAddrs = append(c.ExternalAddrs, addr)
		return nil
	}
}

// WithExternalIP sets an arbitrary ip/port for broadcasting to swarm
func WithExternalIP(ip string, port int) Option {
	externalAddr := fmt.Sprintf("/ip4/%s/tcp/%d", ip, port)
	return WithExternalAddr(externalAddr)
}

// WithWebSocketExternalIP sets an arbitrary ip/port for broadcasting ws path to swarm
func WithWebSocketExternalIP(ip string, port int) Option {
	externalAddr := fmt.Sprintf("/ip4/%s/tcp/%d/ws", ip, port)
	return WithExternalAddr(externalAddr)
}

// WithClientOnlyDHT sets whether or not the DHT will be put into client/server mode
// client mode means it will not serve requests on the DHT
func WithClientOnlyDHT(isClientOnly bool) Option {
	return func(c *Config) error {
		c.ClientOnlyDHT = isClientOnly
		return nil
	}
}

// WithLibp2pOptions allows for additional libp2p options to be passed in
func WithLibp2pOptions(opts ...libp2p.Option) Option {
	return func(c *Config) error {
		c.AdditionalP2POptions = opts
		return nil
	}
}

func WithBitswapOptions(opts ...bitswap.Option) Option {
	return func(c *Config) error {
		c.BitswapOptions = opts
		return nil
	}
}

// WithTransports should only be used with a libp2p.Transport option
// but cannot detect that using typing
func WithTransports(transports ...libp2p.Option) Option {
	return func(c *Config) error {
		c.Transports = transports
		return nil
	}
}

func stringToIPNet(str string) (*net.IPNet, error) {
	_, n, err := net.ParseCIDR(str)
	if err != nil {
		return nil, fmt.Errorf("error parsing %s: %v", str, err)
	}
	return n, nil
}
