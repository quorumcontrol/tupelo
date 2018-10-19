package p2p

import (
	"context"
	"crypto/ecdsa"
	"fmt"

	ds "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-datastore"
	dsync "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-datastore/sync"
	logging "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-log"
	"github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p"
	circuit "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-circuit"
	libp2pcrypto "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-crypto"
	host "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-host"
	dht "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p-kad-dht"
	rhost "github.com/ipsn/go-ipfs/gxlibs/github.com/libp2p/go-libp2p/p2p/host/routed"
	ma "github.com/ipsn/go-ipfs/gxlibs/github.com/multiformats/go-multiaddr"
)

var log = logging.Logger("libp2play")

type Host struct {
	host host.Host
}

func p2pPrivateFromEcdsaPrivate(key *ecdsa.PrivateKey) (libp2pcrypto.PrivKey, error) {
	return libp2pcrypto.UnmarshalSecp256k1PrivateKey(key.D.Bytes())
}

func NewHost(ctx context.Context, privateKey *ecdsa.PrivateKey, port int) (Host, error) {
	priv, err := p2pPrivateFromEcdsaPrivate(privateKey)
	if err != nil {
		return Host{}, err
	}
	// Generate a key pair for this host. We will use it at least
	// to obtain a valid host ID.

	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(fmt.Sprintf("/ip4/0.0.0.0/tcp/%d", port)),
		libp2p.Identity(priv),
		libp2p.DefaultTransports,
		libp2p.DefaultMuxers,
		libp2p.DefaultSecurity,
		libp2p.NATPortMap(),
		libp2p.EnableRelay(circuit.OptActive, circuit.OptHop),
	}
	basicHost, err := libp2p.New(ctx, opts...)
	if err != nil {
		return Host{}, err
	}
	// Construct a datastore (needed by the DHT). This is just a simple, in-memory thread-safe datastore.
	dstore := dsync.MutexWrap(ds.NewMapDatastore())
	// Make the DHT
	dht := dht.NewDHT(ctx, basicHost, dstore)
	// Make the routed host
	routedHost := rhost.Wrap(basicHost, dht)

	return Host{host: routedHost}, nil
}

func (h *Host) Addresses() []ma.Multiaddr {
	hostAddr, _ := ma.NewMultiaddr(fmt.Sprintf("/ipfs/%s", h.host.ID().Pretty()))
	addrs := make([]ma.Multiaddr, 0)
	for _, addr := range h.host.Addrs() {
		addrs = append(addrs, addr.Encapsulate(hostAddr))
	}
	return addrs
}
