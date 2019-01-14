package ipfs

import (
	"context"
	"fmt"
	"os"

	"github.com/ipsn/go-ipfs/core"
	"github.com/ipsn/go-ipfs/core/coreapi"
	coreiface "github.com/ipsn/go-ipfs/core/coreapi/interface"
	config "github.com/ipsn/go-ipfs/gxlibs/github.com/ipfs/go-ipfs-config"
	"github.com/ipsn/go-ipfs/plugin/loader"
	"github.com/ipsn/go-ipfs/repo/fsrepo"
)

const (
	nBitsForKeypairDefault = 2048
)

func StartIpfs(ctx context.Context) (coreiface.DagAPI, error) {
	repoPath := "./storage"
	plugins, err := loader.NewPluginLoader("")

	if err := plugins.Initialize(); err != nil {
		return nil, fmt.Errorf("error initializing plugins: %v", err)
	}
	if err := plugins.Run(); err != nil {
		return nil, fmt.Errorf("error running plugins: %v", err)
	}

	if !fsrepo.IsInitialized(repoPath) {
		conf, err := config.Init(os.Stdout, nBitsForKeypairDefault)
		if err != nil {
			return nil, fmt.Errorf("error initializing file: %v", err)
		}
		conf.Addresses.Swarm = []string{"/ip4/0.0.0.0/tcp/4002"}

		for _, profile := range []string{"server", "badgerds"} {
			transformer, ok := config.Profiles[profile]
			if !ok {
				return nil, fmt.Errorf("invalid configuration profile: %s", "server")
			}

			if err := transformer.Transform(conf); err != nil {
				return nil, fmt.Errorf("error transforming: %v", err)
			}
		}

		if err := fsrepo.Init(repoPath, conf); err != nil {
			return nil, fmt.Errorf("error initializng fsrepo: %v", err)
		}
	}

	repo, err := fsrepo.Open(repoPath)
	if err != nil {
		return nil, fmt.Errorf("error opening repo: %v", err)
	}

	ncfg := &core.BuildCfg{
		Repo:      repo,
		Online:    true,
		Permanent: true,
		Routing:   core.DHTOption,
	}

	node, err := core.NewNode(ctx, ncfg)
	if err != nil {
		return nil, fmt.Errorf("error new node: %v", err)
	}

	api, err := coreapi.NewCoreAPI(node)
	if err != nil {
		return nil, fmt.Errorf("error creating node: %v", err)
	}
	return api.Dag(), nil
}
