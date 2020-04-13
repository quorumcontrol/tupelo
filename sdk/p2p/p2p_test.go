package p2p

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	ds "github.com/ipfs/go-datastore"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	"github.com/libp2p/go-libp2p"
	circuit "github.com/libp2p/go-libp2p-circuit"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-tcp-transport"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func libP2PNodeGenerator(ctx context.Context, t *testing.T) Node {
	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	host, err := NewLibP2PHost(ctx, key, 0)

	require.Nil(t, err)
	require.NotNil(t, host)

	return host
}

func TestHost(t *testing.T) {
	NodeTests(t, libP2PNodeGenerator)
}

func TestP2pBroadcastIp(t *testing.T) {
	startEnv, startEnvOk := os.LookupEnv("TUPELO_PUBLIC_IP")
	defer func() {
		if startEnvOk {
			os.Setenv("TUPELO_PUBLIC_IP", startEnv)
		} else {
			os.Unsetenv("TUPELO_PUBLIC_IP")
		}
	}()
	os.Setenv("TUPELO_PUBLIC_IP", "1.1.1.1")
	testHost := libP2PNodeGenerator(context.Background(), t)
	found := false
	for _, addr := range testHost.Addresses() {
		if strings.HasPrefix(addr.String(), "/ip4/1.1.1.1/") {
			found = true
		}
	}
	require.True(t, found)
}

func TestWithExternalAddrs(t *testing.T) {
	c := &Config{}
	err := applyOptions(c, WithExternalIP("1.1.1.1", 53))
	require.Nil(t, err)
	err = applyOptions(c, WithWebSocketExternalIP("1.1.1.1", 80))
	require.Nil(t, err)
	err = applyOptions(c, WithExternalIP("2.2.2.2", 53))
	require.Nil(t, err)
	err = applyOptions(c, WithWebSocketExternalIP("2.2.2.2", 80))
	require.Nil(t, err)
	require.ElementsMatch(t, c.ExternalAddrs, []string{
		"/ip4/1.1.1.1/tcp/53",
		"/ip4/1.1.1.1/tcp/80/ws",
		"/ip4/2.2.2.2/tcp/53",
		"/ip4/2.2.2.2/tcp/80/ws",
	})
}

func TestWithTransports(t *testing.T) {
	c := &Config{}
	transport := libp2p.Transport(tcp.NewTCPTransport)
	err := applyOptions(c, WithTransports(transport))
	require.Nil(t, err)
	require.Len(t, c.Transports, 1)
	// for some reason equality assertion doesn't work.
}

func TestNewRelayLibP2PHost(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.Nil(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h, err := NewRelayLibP2PHost(ctx, key, 0)
	require.Nil(t, err)
	require.NotNil(t, h)
}

func TestNewHostFromOptions(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	t.Run("it works with no options", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		h, err := NewHostFromOptions(ctx)
		require.Nil(t, err)
		require.NotNil(t, h)
	})

	t.Run("it works with more esoteric options", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		cm := connmgr.NewConnManager(20, 100, 20*time.Second)

		bs := blockstore.NewBlockstore(ds.NewMapDatastore())

		h, err := NewHostFromOptions(
			ctx,
			WithKey(key),
			WithAutoRelay(true),
			WithSegmenter([]byte("my secret password")),
			WithPubSubRouter("floodsub"),
			WithRelayOpts(circuit.OptHop),
			WithLibp2pOptions(libp2p.ConnectionManager(cm)),
			WithClientOnlyDHT(true),
			WithBlockstore(bs),
		)
		require.Nil(t, err)
		require.NotNil(t, h)
		require.Equal(t, bs, h.blockstore)
	})
}
func TestUnmarshal31ByteKey(t *testing.T) {
	// we kept having failures that looked like lack of entropy, but
	// were instead just a normal case where a 31 byte big.Int was generated as a
	// private key from the crypto library. This is one such key:
	keyHex := "0x0092f4d28a01ad9432b4cb9ffb1ecf5628bff465a231b86c9a5b739ead0b3bb5"
	keyBytes, err := hexutil.Decode(keyHex)
	require.Nil(t, err)
	key, err := crypto.ToECDSA(keyBytes)
	require.Nil(t, err)
	libP2PKey, err := p2pPrivateFromEcdsaPrivate(key)
	require.Nil(t, err)

	libP2PPub, err := libP2PKey.GetPublic().Raw()
	require.Nil(t, err)

	assert.Equal(t, crypto.CompressPubkey(&key.PublicKey), libP2PPub)
}

func TestWaitForDiscovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h1, err := NewHostFromOptions(ctx, WithDiscoveryNamespaces("one", "two"))
	require.Nil(t, err)

	h2, err := NewHostFromOptions(ctx, WithDiscoveryNamespaces("one"))
	require.Nil(t, err)

	_, err = h1.Bootstrap(bootstrapAddresses(h2))
	require.Nil(t, err)

	_, err = h2.Bootstrap(bootstrapAddresses(h1))
	require.Nil(t, err)

	err = h1.WaitForBootstrap(1, 10*time.Second)
	require.Nil(t, err)

	// one should already be fine and not wait at all (because of bootstrap)
	err = h1.WaitForDiscovery("one", 1, 2*time.Second)
	require.Nil(t, err)
}

func TestNewDiscoverers(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	h1, err := NewHostFromOptions(ctx)
	require.Nil(t, err)

	h2, err := NewHostFromOptions(ctx)
	require.Nil(t, err)

	_, err = h1.Bootstrap(bootstrapAddresses(h2))
	require.Nil(t, err)

	err = h1.WaitForBootstrap(1, 10*time.Second)
	require.Nil(t, err)

	ns := "totally-new"

	err = h1.StartDiscovery(ns)
	require.Nil(t, err)

	defer h1.StopDiscovery(ns)

	err = h2.StartDiscovery(ns)
	require.Nil(t, err)
	defer h2.StopDiscovery(ns)

	err = h2.WaitForDiscovery(ns, 1, 2*time.Second)
	require.Nil(t, err)
}

func TestPeerIDToPubkeyConversions(t *testing.T) {
	keyBytes, err := hexutil.Decode("0x133f18ab84b61b2f217e972d1a5abe9c3a5ae8b52b13b4c4635bb69cbd5212b0")
	require.Nil(t, err)

	key, err := crypto.ToECDSA(keyBytes)
	require.Nil(t, err)

	peerID, err := PeerFromEcdsaKey(&key.PublicKey)
	require.Nil(t, err)
	require.Equal(t, peerID.Pretty(), "16Uiu2HAmQeLpoMMcZr1hLH6Kw53xFE5Xm8ri4ckGk5vL63sJLLBZ")

	convertedPubkey, err := EcdsaKeyFromPeer(peerID)
	require.Nil(t, err)
	require.Equal(t, crypto.FromECDSAPub(convertedPubkey), crypto.FromECDSAPub(&key.PublicKey))
}

func TestConnectAndDisconnect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bh, err := NewHostFromOptions(ctx)
	require.Nil(t, err)

	h1, err := NewHostFromOptions(ctx)
	require.Nil(t, err)

	h2, err := NewHostFromOptions(ctx)
	require.Nil(t, err)

	_, err = h1.Bootstrap(bootstrapAddresses(bh))
	require.Nil(t, err)

	_, err = h2.Bootstrap(bootstrapAddresses(bh))
	require.Nil(t, err)

	err = h1.WaitForBootstrap(1, 10*time.Second)
	require.Nil(t, err)

	err = h2.WaitForBootstrap(1, 10*time.Second)
	require.Nil(t, err)

	// Seems h1 <=> h2 are already connected in this test setup
	err = h1.Disconnect(ctx, h2.PublicKey())
	require.Nil(t, err)
	require.Len(t, h1.host.Network().Peers(), 1)
	time.Sleep(10 * time.Millisecond) // Give disconnect a brief moment to resolve on other host
	require.Len(t, h2.host.Network().Peers(), 1)

	err = h1.Connect(ctx, h2.PublicKey())
	require.Nil(t, err)
	require.Len(t, h1.host.Network().Peers(), 2)
	require.Len(t, h2.host.Network().Peers(), 2)

	err = h2.Disconnect(ctx, h1.PublicKey())
	require.Nil(t, err)
	require.Len(t, h2.host.Network().Peers(), 1)
	time.Sleep(10 * time.Millisecond) // Give disconnect a brief moment to resolve on other host
	require.Len(t, h1.host.Network().Peers(), 1)
}
