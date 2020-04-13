package nodebuilder

import (
	"context"
	"fmt"
	"strings"

	"github.com/quorumcontrol/tupelo/sdk/gossip/types"
	"github.com/quorumcontrol/tupelo/sdk/p2p"
)

const localConfigName = "local-network"

type LocalNetwork struct {
	Builders        []*NodeBuilder
	Nodes           []p2p.Node
	ClientNode      p2p.Node
	BootstrapNode   p2p.Node
	BootstrapAddrrs []string
	NotaryGroup     *types.NotaryGroup
}

func NewLocalNetwork(ctx context.Context, namespace string, keys []*PrivateKeySet, ngConfig *types.Config) (*LocalNetwork, error) {
	boot, err := p2p.NewHostFromOptions(ctx)
	if err != nil {
		return nil, fmt.Errorf("error creating host from options: %v", err)
	}

	clientNode, err := p2p.NewHostFromOptions(ctx)
	if err != nil {
		return nil, fmt.Errorf("error creating host from options: %v", err)
	}

	ln := &LocalNetwork{
		BootstrapNode:   boot,
		ClientNode:      clientNode,
		BootstrapAddrrs: BootstrapAddresses(boot),
	}

	signers := make([]types.PublicKeySet, len(keys))
	for i, keySet := range keys {
		public := types.PublicKeySet{
			DestKey: &keySet.DestKey.PublicKey,
			VerKey:  keySet.SignKey.MustVerKey(),
		}
		signers[i] = public
	}
	ngConfig.Signers = signers

	configs := make([]*Config, len(signers))
	for i, keySet := range keys {
		hsc := &HumanStorageConfig{
			Kind: "memory",
		}

		bstore, err := hsc.ToBlockstore()
		if err != nil {
			return nil, fmt.Errorf("error setting up badger blockstore: %v", err)
		}

		dstore, err := hsc.ToDatastore()
		if err != nil {
			return nil, fmt.Errorf("error setting up badger datastore: %v", err)
		}

		configs[i] = &Config{
			NotaryGroupConfig: ngConfig,
			PrivateKeySet:     keySet,
			BootstrapNodes:    ln.BootstrapAddrrs,
			Blockstore:        bstore,
			Datastore:         dstore,
		}
	}

	for _, c := range configs {
		nb := &NodeBuilder{Config: c}
		err := nb.Start(ctx)
		if err != nil {
			return nil, fmt.Errorf("error setting up nodebuilder: %v", err)
		}
		ln.Builders = append(ln.Builders, nb)
		ln.Nodes = append(ln.Nodes, nb.Host())
	}

	group := types.NewNotaryGroupFromConfig(ngConfig)

	for _, keySet := range signers {
		signer := types.NewRemoteSigner(keySet.DestKey, keySet.VerKey)
		group.AddSigner(signer)
	}
	ln.NotaryGroup = group

	return ln, nil
}

func (ln *LocalNetwork) BootstrappedP2PNode(ctx context.Context, opts ...p2p.Option) (p2p.Node, error) {
	host, err := p2p.NewHostFromOptions(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("error creating host: %v", err)
	}
	if _, err = host.Bootstrap(ln.BootstrapAddrrs); err != nil {
		return nil, fmt.Errorf("error bootstrapping: %v", err)
	}
	return host, nil
}

func BootstrapAddresses(bootstrapHost p2p.Node) []string {
	addresses := bootstrapHost.Addresses()
	for _, addr := range addresses {
		addrStr := addr.String()
		if strings.Contains(addrStr, "127.0.0.1") {
			return []string{addrStr}
		}
	}
	return nil
}
