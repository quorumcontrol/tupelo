package p2p

import (
	"context"
	"math/rand"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p-core/network"

	logging "github.com/ipfs/go-log"
	host "github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/routing"
	dht "github.com/libp2p/go-libp2p-kad-dht"
	ma "github.com/multiformats/go-multiaddr"
)

var (
	LocalBootstrap = []string{
		"/ip4/127.0.0.1/tcp/10000/ipfs/16Uiu2HAmTfa6xM13GKVrx6J6WkXxV26CmifQyJVwJZTDHSmMgBu3",
	}
	defaultBootstrapNodes = []string{
		"/ip4/18.196.112.81/tcp/34001/ipfs/16Uiu2HAmJXuoQMRqg4bShcBTczUMn8zMyCvXAPuefCtqZb21iih8",
		"/ip4/34.231.17.217/tcp/34001/ipfs/16Uiu2HAmLos2gmQLkVkiYF3JJBDW2WqZxCHoMb2fLmo77a2tqExF",
	}

	// see https://github.com/filecoin-project/go-filecoin/blob/master/net/bootstrap.go
	tupeloBootstrapConfig = dht.BootstrapConfig{
		// Recommended initial options from issue #1947
		Queries: 2,
		Period:  5 * time.Minute,
		Timeout: time.Minute,
	}
	logBootstrap = logging.Logger("p2p.bootstrap")
)

// Bootstrapper attempts to keep the p2p host connected to the network
// by keeping a minimum threshold of connections. If the threshold isn't met it
// connects to a random subset of the bootstrap peers. It does not use peer routing
// to discover new peers. To stop a Bootstrapper cancel the context passed in Start()
// or call Stop().
type Bootstrapper struct {
	// Config
	// MinPeerThreshold is the number of connections it attempts to maintain.
	MinPeerThreshold int
	// Peers to connect to if we fall below the threshold.
	bootstrapPeers []peer.AddrInfo
	// Period is the interval at which it periodically checks to see
	// if the threshold is maintained.
	Period time.Duration
	// ConnectionTimeout is how long to wait before timing out a connection attempt.
	ConnectionTimeout time.Duration

	// Dependencies
	h host.Host
	d network.Dialer
	r routing.Routing
	// Does the work. Usually Bootstrapper.bootstrap. Argument is a slice of
	// currently-connected peers (so it won't attempt to reconnect).
	Bootstrap func([]peer.ID)

	// Bookkeeping
	ticker         *time.Ticker
	ctx            context.Context
	cancel         context.CancelFunc
	dhtBootStarted bool
}

// NewBootstrapper returns a new Bootstrapper that will attempt to keep connected
// to the network by connecting to the given bootstrap peers.
func NewBootstrapper(bootstrapPeers []peer.AddrInfo, h host.Host, d network.Dialer, r routing.Routing, minPeer int, period time.Duration) *Bootstrapper {
	b := &Bootstrapper{
		MinPeerThreshold:  minPeer,
		bootstrapPeers:    bootstrapPeers,
		Period:            period,
		ConnectionTimeout: 20 * time.Second,

		h: h,
		d: d,
		r: r,
	}
	b.Bootstrap = b.bootstrap
	return b
}

// Start starts the Bootstrapper bootstrapping. Cancel `ctx` or call Stop() to stop it.
func (b *Bootstrapper) Start(ctx context.Context) {
	b.ctx, b.cancel = context.WithCancel(ctx)
	b.ticker = time.NewTicker(b.Period)

	go func() {
		defer b.ticker.Stop()
		// do one to start
		b.Bootstrap(b.d.Peers())

		for {
			select {
			case <-b.ctx.Done():
				return
			case <-b.ticker.C:
				b.Bootstrap(b.d.Peers())
			}
		}
	}()
}

// Stop stops the Bootstrapper.
func (b *Bootstrapper) Stop() {
	if b.cancel != nil {
		b.cancel()
	}
}

// Close implements io.Closer in order to provide
// the same interface as previously.
func (b *Bootstrapper) Close() error {
	b.Stop()
	return nil
}

// bootstrap does the actual work. If the number of connected peers
// has fallen below b.MinPeerThreshold it will attempt to connect to
// a random subset of its bootstrap peers.
func (b *Bootstrapper) bootstrap(currentPeers []peer.ID) {
	peersNeeded := b.MinPeerThreshold - len(currentPeers)
	if peersNeeded < 1 {
		return
	}

	ctx, cancel := context.WithTimeout(b.ctx, b.ConnectionTimeout)
	var wg sync.WaitGroup
	defer func() {
		wg.Wait()
		// After connecting to bootstrap peers, bootstrap the DHT.
		// DHT Bootstrap is a persistent process so only do this once.
		if !b.dhtBootStarted {
			b.dhtBootStarted = true
			err := b.bootstrapRouting()
			if err != nil {
				logBootstrap.Warningf("got error trying to bootstrap Routing: %s. Peer discovery may suffer.", err.Error())
			}
		}
		cancel()
	}()

	peersAttempted := 0
	for _, i := range rand.Perm(len(b.bootstrapPeers)) {
		pinfo := b.bootstrapPeers[i]
		// Don't try to connect to an already connected peer.
		if hasPID(currentPeers, pinfo.ID) {
			continue
		}

		wg.Add(1)
		go func() {
			if err := b.h.Connect(ctx, pinfo); err != nil {
				logBootstrap.Errorf("got error trying to connect to bootstrap node %+v: %s", pinfo, err.Error())
			}
			wg.Done()
		}()
		peersAttempted++
		if peersAttempted == peersNeeded {
			return
		}
	}
	logBootstrap.Warningf("not enough bootstrap nodes to maintain %d connections (current connections: %d)", b.MinPeerThreshold, len(currentPeers))
}

func hasPID(pids []peer.ID, pid peer.ID) bool {
	for _, p := range pids {
		if p == pid {
			return true
		}
	}
	return false
}

func (b *Bootstrapper) bootstrapRouting() error {
	dht, ok := b.r.(*dht.IpfsDHT)
	if !ok {
		// No bootstrapping to do exit quietly.
		return nil
	}

	return dht.BootstrapWithConfig(b.ctx, tupeloBootstrapConfig)
}

// BootstrapNodes returns a slice of comma-saparated values from environment variable TUPELO_BOOTSTRAP_NODES
// or the default bootstrapNodes if the environment variable is not set.
func BootstrapNodes() []string {
	if envSpecifiedNodes, ok := os.LookupEnv("TUPELO_BOOTSTRAP_NODES"); ok {
		return strings.Split(envSpecifiedNodes, ",")
	}

	return defaultBootstrapNodes
}

func convertPeers(peers []string) []peer.AddrInfo {
	pinfos := make([]peer.AddrInfo, len(peers))
	for i, p := range peers {
		maddr := ma.StringCast(p)
		ai, err := peer.AddrInfoFromP2pAddr(maddr)
		if err != nil {
			log.Fatal(err)
		}
		pinfos[i] = *ai
	}
	return pinfos
}
