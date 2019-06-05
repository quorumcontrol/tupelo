package nodebuilder

import (
	"context"
	"crypto/ecdsa"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/quorumcontrol/tupelo-go-sdk/tracing"

	"github.com/quorumcontrol/tupelo/gossip3/actors"

	"github.com/AsynkronIT/protoactor-go/actor"

	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/remote"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"

	"github.com/libp2p/go-libp2p"
	"github.com/quorumcontrol/tupelo-go-sdk/p2p"

	circuit "github.com/libp2p/go-libp2p-circuit"
	connmgr "github.com/libp2p/go-libp2p-connmgr"
	peer "github.com/libp2p/go-libp2p-peer"
	"github.com/quorumcontrol/storage"
	"github.com/shibukawa/configdir"
)

const remoteNetworkNamespace = "distributed-network"

type NodeBuilder struct {
	Config *Config
	host   p2p.Node
}

func (nb *NodeBuilder) Start(ctx context.Context) error {
	nb.startTracing()

	if nb.Config.BootstrapOnly {
		return nb.startBootstrap(ctx)
	}

	return nb.startSigner(ctx)
}

func (nb *NodeBuilder) startTracing() {
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

func (nb *NodeBuilder) startSigner(ctx context.Context) error {
	remote.Start()
	localKeys := nb.Config.PrivateKeySet
	localSigner := types.NewLocalSigner(&localKeys.DestKey.PublicKey, localKeys.SignKey)

	storagePath := nb.configDir(remoteNetworkNamespace)
	currentPath := signerCurrentPath(storagePath, localSigner)

	badgerCurrent, err := storage.NewBadgerStorage(currentPath)
	if err != nil {
		return fmt.Errorf("error creating storage: %v", err)
	}

	group := nb.setupNotaryGroup(localSigner)

	cm := connmgr.NewConnManager(len(group.Signers)*2, 900, 20*time.Second)
	for _, s := range group.Signers {
		id, err := p2p.PeerFromEcdsaKey(s.DstKey)
		if err != nil {
			panic(fmt.Sprintf("error getting peer from ecdsa key: %v", err))
		}
		cm.Protect(id, "signer")
	}

	p2pHost, err := p2pNodeWithOpts(ctx, nb.Config.PrivateKeySet.DestKey, nb.Config.Port, p2p.WithLibp2pOptions(libp2p.ConnectionManager(cm)))
	if err != nil {
		return fmt.Errorf("error setting up p2p host: %v", err)
	}
	if _, err = p2pHost.Bootstrap(nb.bootstrapNodes()); err != nil {
		return fmt.Errorf("failed to bootstrap: %s", err)
	}

	nb.host = p2pHost

	remote.NewRouter(p2pHost)

	act, err := actor.SpawnNamed(actors.NewTupeloNodeProps(&actors.TupeloConfig{
		Self:              localSigner,
		NotaryGroup:       group,
		CurrentStateStore: badgerCurrent,
		PubSubSystem:      remote.NewNetworkPubSub(p2pHost),
	}), syncerActorName(localSigner))
	if err != nil {
		panic(fmt.Sprintf("error spawning: %v", err))
	}

	localSigner.Actor = act
	return nil
}

func (nb *NodeBuilder) setupNotaryGroup(local *types.Signer) *types.NotaryGroup {

	group := types.NewNotaryGroupFromConfig(nb.Config.NotaryGroupConfig)

	if local != nil {
		group.AddSigner(local)
	}

	for _, keySet := range nb.Config.Signers {
		signer := types.NewRemoteSigner(keySet.DestKey, keySet.VerKey)
		if local != nil {
			signer.Actor = actor.NewPID(signer.ActorAddress(local.DstKey), syncerActorName(signer))
		}
		group.AddSigner(signer)
	}

	return group
}

func syncerActorName(signer *types.Signer) string {
	return "tupelo-" + signer.ID
}

func (nb *NodeBuilder) startBootstrap(ctx context.Context) error {
	cm := connmgr.NewConnManager(4915, 7372, 30*time.Second)

	host, err := p2pNodeWithOpts(
		ctx,
		nb.Config.PrivateKeySet.DestKey,
		nb.Config.Port,
		p2p.WithLibp2pOptions(libp2p.ConnectionManager(cm)),
		p2p.WithRelayOpts(circuit.OptHop),
	)
	if err != nil {
		return fmt.Errorf("Could not start bootstrap node, %v", err)
	}

	nb.host = host

	boostrappers, err := nb.bootstrapNodesWithoutSelf()
	if err != nil {
		return fmt.Errorf("error getting bootstrap nodes: %v", err)
	}

	if len(boostrappers) > 0 {
		if _, err = host.Bootstrap(boostrappers); err != nil {
			return fmt.Errorf("bootstrapping failed: %s", err)
		}
	}

	return nil
}

func (nb *NodeBuilder) bootstrapNodes() []string {
	if len(nb.Config.BootstrapNodes) > 0 {
		return nb.Config.BootstrapNodes
	}
	return p2p.BootstrapNodes()
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

func p2pNodeWithOpts(ctx context.Context, ecdsaKey *ecdsa.PrivateKey, port int, addlOpts ...p2p.Option) (p2p.Node, error) {
	opts := []p2p.Option{
		p2p.WithKey(ecdsaKey),
		p2p.WithDiscoveryNamespaces("tupelo-transaction-gossipers"),
		p2p.WithListenIP("0.0.0.0", port),
	}
	if hostIP, ok := os.LookupEnv("TUPELO_PUBLIC_IP"); ok {
		opts = append(opts, p2p.WithExternalIP(hostIP, port))
	}
	return p2p.NewHostFromOptions(ctx, append(opts, addlOpts...)...)
}

func (nb *NodeBuilder) configDir(namespace string) string {
	conf := configdir.New("tupelo", filepath.Join(nb.Config.Namespace, namespace))
	folders := conf.QueryFolders(configdir.Global)
	if err := os.MkdirAll(folders[0].Path, 0700); err != nil {
		panic(err)
	}
	return folders[0].Path
}

func signerCurrentPath(storagePath string, signer *types.Signer) (path string) {
	path = filepath.Join(storagePath, signer.ID+"-current")
	if err := os.MkdirAll(path, 0755); err != nil {
		panic(err)
	}
	return
}
