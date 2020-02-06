package nodebuilder

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip/types"
	"github.com/quorumcontrol/tupelo-go-sdk/tracing"
	"github.com/quorumcontrol/tupelo/gossip"
	"github.com/quorumcontrol/tupelo/proxy"

	"github.com/ipfs/go-bitswap"
	logging "github.com/ipfs/go-log"
	"github.com/libp2p/go-libp2p"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"

	circuit "github.com/libp2p/go-libp2p-circuit"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/shibukawa/configdir"
)

const MULTIADDR_WSS_CODE = 0x01DE

func init() {
	// Code 0 means not yet registered
	if ma.ProtocolWithCode(MULTIADDR_WSS_CODE).Code == 0 {
		ma.AddProtocol(ma.Protocol{
			Name:  "wss",
			Code:  MULTIADDR_WSS_CODE,
			VCode: ma.CodeToVarint(MULTIADDR_WSS_CODE),
		})
	}
}

var logger = logging.Logger("nodebuilder")

type NodeBuilder struct {
	Config      *Config
	host        p2p.Node
	signerActor *actor.PID
}

func (nb *NodeBuilder) Host() p2p.Node {
	return nb.host
}

func (nb *NodeBuilder) Actor() *actor.PID {
	return nb.signerActor
}

func (nb *NodeBuilder) NotaryGroup() (*types.NotaryGroup, error) {
	return nb.Config.NotaryGroupConfig.NotaryGroup(nil)
}

func (nb *NodeBuilder) Start(ctx context.Context) error {
	err := nb.configAssertions()
	if err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		if err := nb.Stop(); err != nil {
			logger.Errorf("error stopping: %w", err)
		}
	}()

	nb.StartTracing()

	var host *p2p.LibP2PHost
	if nb.Config.BootstrapOnly {
		h, e := nb.startBootstrap(ctx)
		host = h
		err = e
	} else {
		h, e := nb.startSigner(ctx)
		host = h
		err = e
	}
	if err != nil {
		return fmt.Errorf("error starting: %w", err)
	}

	if nb.Config.SecureWebSocketDomain != "" {
		if err := nb.startSecureWebSocketProxy(ctx, host); err != nil {
			return fmt.Errorf("error starting secure websocket proxy: %v", err)
		}
	}

	return nil
}

func (nb *NodeBuilder) Stop() error {
	if nb.Config.TracingSystem == JaegerTracing {
		tracing.StopJaeger()
	}

	if nb.signerActor != nil {
		actor.EmptyRootContext.Stop(nb.Actor())
	}

	return nil
}

func (nb *NodeBuilder) startSecureWebSocketProxy(ctx context.Context, host *p2p.LibP2PHost) error {
	swsd := nb.Config.SecureWebSocketDomain

	dialStr, err := wsAddrFromMultiAddrs(host.Addresses())
	if err != nil {
		return fmt.Errorf("error getting wsAddr: %v", err)
	}
	logger.Infof("Starting TLS proxy for %s connecting to %s", swsd, dialStr)
	p := proxy.Server{
		BindDomain: swsd,
		Backend: proxy.Backend{
			Addr:           dialStr,
			ConnectTimeout: 5000, // 5 seconds
		},
		CertDirectory: nb.Config.CertificateCache,
	}
	go func() {
		err := p.Run()
		if err != nil {
			logger.Errorf("error running: %v", err)
		}
	}()
	return nil
}

func (nb *NodeBuilder) configAssertions() error {
	conf := nb.Config
	if !conf.BootstrapOnly && conf.NotaryGroupConfig == nil {
		return fmt.Errorf("error: must specify a NotaryGroupConfig (there's a DefaultConfig() helper in tupelo-go-sdk)")
	}

	if len(conf.BootstrapNodes) == 0 {
		return fmt.Errorf("you must explicitly provide bootstrap nodes")
	}

	return nil
}

func (nb *NodeBuilder) StartTracing() {
	switch nb.Config.TracingSystem {
	case JaegerTracing:
		own, err := nb.ownPeerID()
		if err != nil {
			panic(fmt.Errorf("error getting own ID: %v", err))
		}
		tracing.StartJaeger(own.String())
	case ElasticTracing:
		tracing.StartElastic()
	}
}

func (nb *NodeBuilder) startSigner(ctx context.Context) (*p2p.LibP2PHost, error) {
	localKeys := nb.Config.PrivateKeySet
	localSigner := types.NewLocalSigner(&localKeys.DestKey.PublicKey, localKeys.SignKey)
	group, err := nb.Config.NotaryGroupConfig.NotaryGroup(localSigner)
	if err != nil {
		return nil, fmt.Errorf("error generating notary group: %w", err)
	}

	cm := connmgr.NewConnManager(len(group.Signers)*5, 900, 20*time.Second)
	for _, s := range group.Signers {
		id, err := p2p.PeerFromEcdsaKey(s.DstKey)
		if err != nil {
			panic(fmt.Sprintf("error getting peer from ecdsa key: %v", err))
		}
		cm.Protect(id, "signer")
	}

	blockstorage, err := nb.Config.Storage.ToBlockstore() // defaults to memory
	if err != nil {
		return nil, fmt.Errorf("error converting to datastore: %w", err)
	}

	p2pStore, err := nb.Config.P2PStorage.ToDatastore() // defaults to memory
	if err != nil {
		return nil, fmt.Errorf("error converting to datastore: %w", err)
	}

	p2pNode, bitswapper, err := p2p.NewHostAndBitSwapPeer(
		ctx,
		append(nb.defaultP2POptions(ctx),
			p2p.WithLibp2pOptions(libp2p.ConnectionManager(cm)),
			p2p.WithDatastore(p2pStore),
			p2p.WithBlockstore(blockstorage),
		)...,
	)
	if err != nil {
		return nil, fmt.Errorf("error creating p2p node: %w", err)
	}

	nb.host = p2pNode

	nodeCfg := &gossip.NewNodeOptions{
		P2PNode:     p2pNode,
		SignKey:     localKeys.SignKey,
		NotaryGroup: group,
		DagStore:    bitswapper,
	}

	node, err := gossip.NewNode(ctx, nodeCfg)
	if err != nil {
		return nil, fmt.Errorf("error creating new node: %v", err)
	}

	bootstappers, err := nb.bootstrapNodesWithoutSelf()
	if err != nil {
		return nil, fmt.Errorf("error getting bootstrap nodes: %w", err)
	}

	err = node.Bootstrap(ctx, bootstappers)
	if err != nil {
		return nil, fmt.Errorf("error bootstrapping node: %w", err)
	}

	err = node.Start(ctx)
	if err != nil {
		return nil, fmt.Errorf("error starting node: %v", err)
	}

	nb.signerActor = node.PID()

	return p2pNode, nil
}

func (nb *NodeBuilder) startBootstrap(ctx context.Context) (*p2p.LibP2PHost, error) {
	cm := connmgr.NewConnManager(4915, 7372, 30*time.Second)

	host, err := p2p.NewHostFromOptions(
		ctx,
		append(nb.defaultP2POptions(ctx),
			p2p.WithLibp2pOptions(libp2p.ConnectionManager(cm)),
			p2p.WithRelayOpts(circuit.OptHop),
		)...,
	)
	if err != nil {
		return host, fmt.Errorf("Could not start bootstrap node, %v", err)
	}

	nb.host = host

	bootstappers, err := nb.bootstrapNodesWithoutSelf()
	if err != nil {
		return host, fmt.Errorf("error getting bootstrap nodes: %v", err)
	}

	if len(bootstappers) > 0 {
		if _, err = host.Bootstrap(bootstappers); err != nil {
			return host, fmt.Errorf("bootstrapping failed: %s", err)
		}
	}

	// TODO: not sure we want this here long term, but for now let the
	// bootstrapper help out with gossip pubsub
	group, err := nb.NotaryGroup()
	if err != nil {
		return host, fmt.Errorf("error getting notary group %w", err)
	}
	_, err = host.GetPubSub().Subscribe(group.Config().TransactionTopic)
	if err != nil {
		return host, fmt.Errorf("error subscribing %w", err)
	}
	_, err = host.GetPubSub().Subscribe(group.ID)
	if err != nil {
		return host, fmt.Errorf("error subscribing %w", err)
	}
	return host, nil
}

func (nb *NodeBuilder) bootstrapNodes() []string {
	return nb.Config.BootstrapNodes
}

func (nb *NodeBuilder) bootstrapNodesWithoutSelf() ([]string, error) {
	own, err := nb.ownPeerID()
	if err != nil {
		return nil, fmt.Errorf("error getting own peerID: %v", err)
	}

	var bootstrapWithoutSelf []string

	for _, nodeAddr := range nb.bootstrapNodes() {
		if !strings.Contains(nodeAddr, own.String()) {
			bootstrapWithoutSelf = append(bootstrapWithoutSelf, nodeAddr)
		}
	}

	return bootstrapWithoutSelf, nil
}

func (nb *NodeBuilder) ownPeerID() (peer.ID, error) {
	return p2p.PeerFromEcdsaKey(&nb.Config.PrivateKeySet.DestKey.PublicKey)
}

func (nb *NodeBuilder) defaultP2POptions(ctx context.Context) []p2p.Option {
	opts := []p2p.Option{
		p2p.WithDiscoveryNamespaces(nb.Config.NotaryGroupConfig.ID),
		p2p.WithListenIP("0.0.0.0", nb.Config.Port),
		p2p.WithBitswapOptions(bitswap.ProvideEnabled(false)),
		//TODO: do we want to enable this?
		// p2p.WithPubSubOptions(pubsub.WithStrictSignatureVerification(true), pubsub.WithMessageSigning(true)),
		p2p.WithWebSockets(nb.Config.WebSocketPort),
	}

	if nb.Config.PrivateKeySet != nil && nb.Config.PrivateKeySet.DestKey != nil {
		opts = append(opts, p2p.WithKey(nb.Config.PrivateKeySet.DestKey))
	}

	if nb.Config.PublicIP != "" {
		logger.Debugf("configuring host with public IP: %v and port: %v", nb.Config.PublicIP, nb.Config.Port)
		opts = append(opts,
			p2p.WithExternalIP(nb.Config.PublicIP, nb.Config.Port),
			p2p.WithWebSocketExternalIP(nb.Config.PublicIP, nb.Config.WebSocketPort),
		)
	} else {
		logger.Debug("host has no public IP")
	}

	if nb.Config.SecureWebSocketDomain != "" {
		opts = append(opts, p2p.WithExternalAddr("/dns4/"+nb.Config.SecureWebSocketDomain+"/tcp/443/wss"))
	}

	return opts
}

func configDir(globalNamespace, namespace string) string {
	conf := configdir.New("tupelo", filepath.Join(globalNamespace, namespace))
	folders := conf.QueryFolders(configdir.Global)
	if err := os.MkdirAll(folders[0].Path, 0700); err != nil {
		panic(err)
	}
	return folders[0].Path
}
