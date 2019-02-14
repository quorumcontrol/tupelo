package adapters

import (
	"context"
	"fmt"

	"github.com/ipsn/go-ipfs/core"
	"github.com/ipsn/go-ipfs/plugin/loader"
	"github.com/ipsn/go-ipfs/repo/fsrepo"
	"github.com/quorumcontrol/chaintree/nodestore"
)

const IpldStorageAdapterName = "ipld"

type IpldStorageAdapter struct {
	store *nodestore.IpldStore
	node  *core.IpfsNode
}

func NewIpldStorage(config map[string]interface{}) (*IpldStorageAdapter, error) {
	repoRoot, ok := config["path"].(string)

	if !ok {
		return nil, fmt.Errorf("IPLD requires path in StorageConfig")
	}

	if !fsrepo.IsInitialized(repoRoot) {
		return nil, fmt.Errorf("IPFS node config does not exist at %s, please create it with `ipfs init`", repoRoot)
	}

	daemonLocked, err := fsrepo.LockedByOtherProcess(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("IPFS initialization error: %v", err)
	}

	if daemonLocked {
		return nil, fmt.Errorf("IPFS daemon has lock on %v, please stop it before running tupelo", repoRoot)
	}

	plugins, err := loader.NewPluginLoader("")
	if err != nil {
		return nil, fmt.Errorf("Could not initialize ipfs plugin loader")
	}
	plugins.Initialize()
	plugins.Run()

	repo, err := fsrepo.Open(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("Could not open IPFS repo %s: %v", repoRoot, err)
	}

	nodeConfig := &core.BuildCfg{
		Online:    false,
		Permanent: true,
		Routing:   core.DHTOption,
		Repo:      repo,
	}

	node, err := core.NewNode(context.Background(), nodeConfig)
	if err != nil {
		return nil, fmt.Errorf("Error creating IPFS node: %v", err)
	}

	return &IpldStorageAdapter{
		store: nodestore.NewIpldStore(node),
		node:  node,
	}, nil
}

func (a *IpldStorageAdapter) Store() nodestore.NodeStore {
	return a.store
}

func (a *IpldStorageAdapter) Close() error {
	return a.node.Close()
}
