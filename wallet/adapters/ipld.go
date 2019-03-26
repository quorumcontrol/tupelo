package adapters

import (
	"context"
	"fmt"
	"log"

	ipfsHttpClient "github.com/ipfs/go-ipfs-http-client"
	"github.com/ipsn/go-ipfs/core"
	"github.com/ipsn/go-ipfs/core/coreapi"
	ma "github.com/ipsn/go-ipfs/gxlibs/github.com/multiformats/go-multiaddr"
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
	path, _ := config["path"].(string)
	if len(path) > 0 {
		return NewIpldNodeStorage(config)
	}

	address, _ := config["address"].(string)
	if len(address) > 0 {
		return NewIpldHttpStorage(config)
	}

	return nil, fmt.Errorf("IPLD requires path or address in StorageConfig")
}

func NewIpldHttpStorage(config map[string]interface{}) (*IpldStorageAdapter, error) {
	apiAddr, _ := config["address"].(string)
	if len(apiAddr) == 0 {
		return nil, fmt.Errorf("IPLD http client requires address in StorageConfig")
	}

	apiMaddr, err := ma.NewMultiaddr(apiAddr)
	if err != nil {
		return nil, fmt.Errorf("Error parsing address: %v", err)
	}

	api, err := ipfsHttpClient.NewApi(apiMaddr)

	if err != nil {
		return nil, fmt.Errorf("Error starting ipfs http api client")
	}

	return &IpldStorageAdapter{
		store: nodestore.NewIpldStore(api),
	}, nil
}

func NewIpldNodeStorage(config map[string]interface{}) (*IpldStorageAdapter, error) {
	repoRoot, _ := config["path"].(string)
	if len(repoRoot) == 0 {
		return nil, fmt.Errorf("IPLD node requires path in StorageConfig")
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
	if err = plugins.Initialize(); err != nil {
		log.Printf("failed to initialize ipfs plugins: %s", err)
		// TODO: Enable
		// return nil, fmt.Errorf("failed to initialize ipfs plugins: %s", err)
	}
	if err = plugins.Inject(); err != nil {
		log.Printf("failed to inject ipfs plugins: %s", err)
		// TODO: Enable
		// return nil, fmt.Errorf("failed to inject ipfs plugins: %s", err)
	}

	repo, err := fsrepo.Open(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("Could not open IPFS repo %s: %v", repoRoot, err)
	}

	online := false
	if onlineVal, ok := config["online"]; ok {
		online = onlineVal.(bool)
	}

	nodeConfig := &core.BuildCfg{
		Online:    online,
		Permanent: true,
		Routing:   core.DHTOption,
		Repo:      repo,
	}

	node, err := core.NewNode(context.Background(), nodeConfig)
	if err != nil {
		return nil, fmt.Errorf("Error creating IPFS node: %v", err)
	}

	api, err := coreapi.NewCoreAPI(node)
	if err != nil {
		return nil, fmt.Errorf("Error creating IPFS api: %v", err)
	}

	return &IpldStorageAdapter{
		store: nodestore.NewIpldStore(api),
		node:  node,
	}, nil
}

func (a *IpldStorageAdapter) Store() nodestore.NodeStore {
	return a.store
}

func (a *IpldStorageAdapter) Close() error {
	if a.node != nil {
		return a.node.Close()
	}
	return nil
}
