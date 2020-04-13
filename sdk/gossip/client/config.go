package client

import (
	"context"
	"fmt"
	"time"

	"github.com/ipfs/go-bitswap"
	datastore "github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	blockstore "github.com/ipfs/go-ipfs-blockstore"
	logging "github.com/ipfs/go-log"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/messages/v2/build/go/gossip"
	"github.com/quorumcontrol/messages/v2/build/go/transactions"
	"github.com/quorumcontrol/tupelo/sdk/bls"
	"github.com/quorumcontrol/tupelo/sdk/gossip/client/pubsubinterfaces"
	"github.com/quorumcontrol/tupelo/sdk/gossip/client/pubsubinterfaces/pubsubwrapper"
	"github.com/quorumcontrol/tupelo/sdk/gossip/types"
	"github.com/quorumcontrol/tupelo/sdk/p2p"
	"github.com/quorumcontrol/tupelo/sdk/proof"
)

var DefaultNotaryGroup = "gossip4"

type Config struct {
	Group        *types.NotaryGroup
	Logger       logging.EventLogger
	Pubsub       pubsubinterfaces.Pubsubber
	Storage      datastore.Batching
	Store        nodestore.DagStore
	Validators   []chaintree.BlockValidatorFunc
	Transactors  map[transactions.Transaction_Type]chaintree.TransactorFunc
	OnStartHooks []OnStartHook
	subscriber   *roundSubscriber
}

type Option func(c *Config) error

type OnStartHook func(c *Client) error

func (c *Config) SetDefaults(ctx context.Context) error {
	var err error

	if c.Logger == nil {
		err = c.ApplyOptions(WithLogger(logging.Logger("g4-client")))
		if err != nil {
			return err
		}
	}

	if c.Storage == nil {
		err = c.ApplyOptions(WithMemoryStorage())
		if err != nil {
			return err
		}
	}

	if c.Group == nil {
		err = c.ApplyOptions(WithDefaultNotaryGroup())
		if err != nil {
			return err
		}
	}

	if c.Pubsub == nil && c.Store == nil {
		err = c.ApplyOptions(defaultP2P(ctx))
		if err != nil {
			return err
		}
	}

	if c.subscriber == nil {
		err = c.ApplyOptions(defaultRoundSubscriber())
		if err != nil {
			return err
		}
	}

	if c.Validators == nil {
		err = c.ApplyOptions(WithValidatorsFromGroup(ctx))
		if err != nil {
			return err
		}
	}

	if c.Transactors == nil {
		err = c.ApplyOptions(WithTransactorsFromGroup())
		if err != nil {
			return err
		}
	}

	return nil
}

func (c *Config) ApplyOptions(opts ...Option) error {
	for _, factory := range opts {
		err := factory(c)
		if err != nil {
			return fmt.Errorf("error applying option: %v", err)
		}
	}
	return nil
}

func WithPubsub(v pubsubinterfaces.Pubsubber) Option {
	return func(c *Config) error {
		c.Pubsub = v
		return nil
	}
}

func WithDagStore(v nodestore.DagStore) Option {
	return func(c *Config) error {
		c.Store = v
		return nil
	}
}

func WithLogger(v logging.EventLogger) Option {
	return func(c *Config) error {
		c.Logger = v
		return nil
	}
}

func WithValidators(v []chaintree.BlockValidatorFunc) Option {
	return func(c *Config) error {
		c.Validators = v
		return nil
	}
}

func WithValidatorsFromGroup(ctx context.Context) Option {
	return func(c *Config) error {
		if c.Group == nil {
			return fmt.Errorf("Must set Config.Group for block validators")
		}

		quorumCount := c.Group.QuorumCount()
		signers := c.Group.AllSigners()
		verKeys := make([]*bls.VerKey, len(signers))
		for i, signer := range signers {
			verKeys[i] = signer.VerKey
		}

		proofVerifier := types.GenerateHasValidProof(func(prf *gossip.Proof) (bool, error) {
			return proof.Verify(ctx, prf, quorumCount, verKeys)
		})

		blockValidators, err := c.Group.BlockValidators(ctx)
		if err != nil {
			return fmt.Errorf("error getting block validators: %v", err)
		}

		allValidators := append(blockValidators, proofVerifier)
		return WithValidators(allValidators)(c)
	}
}

func WithTransactors(v map[transactions.Transaction_Type]chaintree.TransactorFunc) Option {
	return func(c *Config) error {
		c.Transactors = v
		return nil
	}
}

func WithTransactorsFromGroup() Option {
	return func(c *Config) error {
		if c.Group == nil {
			return fmt.Errorf("Must set Config.Group for transactors")
		}
		return WithTransactors(c.Group.Config().Transactions)(c)
	}
}

func WithStorage(v datastore.Batching) Option {
	return func(c *Config) error {
		c.Storage = v
		return nil
	}
}

func WithMemoryStorage() Option {
	return WithStorage(dsync.MutexWrap(datastore.NewMapDatastore()))
}

func WithNotaryGroup(v *types.NotaryGroup) Option {
	return func(c *Config) error {
		c.Group = v
		return nil
	}
}

func WithOnStartHook(hook func(c *Client) error) Option {
	return func(c *Config) error {
		if c.OnStartHooks == nil {
			c.OnStartHooks = make([]OnStartHook, 0)
		}
		c.OnStartHooks = append(c.OnStartHooks, hook)
		return nil
	}
}

func WithNotaryGroupToml(notaryGroupToml string) Option {
	return func(c *Config) error {
		ngConfig, err := types.TomlToConfig(notaryGroupToml)
		if err != nil {
			return err
		}
		ng, err := ngConfig.NotaryGroup(nil)
		if err != nil {
			return err
		}
		return WithNotaryGroup(ng)(c)
	}
}

func WithNamedNotaryGroup(name string) Option {
	return func(c *Config) error {
		ngToml, ok := NotaryGroupConfigs[name]
		if !ok {
			return fmt.Errorf("Unknown NotaryGroup named %s", name)
		}
		return WithNotaryGroupToml(ngToml)(c)
	}
}

func WithDefaultNotaryGroup() Option {
	return WithNamedNotaryGroup(DefaultNotaryGroup)
}

func defaultRoundSubscriber() Option {
	return func(c *Config) error {
		if c.Logger == nil {
			return fmt.Errorf("Must set Config.Logger for round subscriber")
		}
		if c.Group == nil {
			return fmt.Errorf("Must set Config.Group for round subscriber")
		}
		if c.Pubsub == nil {
			return fmt.Errorf("Must set Config.Pubsub for round subscriber")
		}
		if c.Store == nil {
			return fmt.Errorf("Must set Config.Store for round subscriber")
		}

		c.subscriber = newRoundSubscriber(c.Logger, c.Group, c.Pubsub, c.Store)
		return nil
	}
}

func defaultP2P(ctx context.Context) Option {
	return func(c *Config) error {
		if c.Group == nil {
			return fmt.Errorf("Must set Config.Group for default p2p")
		}
		if c.Storage == nil {
			return fmt.Errorf("Must set Config.Storage for default p2p")
		}

		bs := blockstore.NewBlockstore(c.Storage)
		bs = blockstore.NewIdStore(bs)

		p2pHost, peer, err := p2p.NewHostAndBitSwapPeer(
			ctx,
			p2p.WithDiscoveryNamespaces(c.Group.ID),
			p2p.WithBitswapOptions(bitswap.ProvideEnabled(false)),
			p2p.WithBlockstore(bs),
		)
		if err != nil {
			return fmt.Errorf("error creating p2p host: %w", err)
		}

		err = WithDagStore(peer)(c)
		if err != nil {
			return err
		}

		err = WithOnStartHook(func(cli *Client) error {
			_, err = p2pHost.Bootstrap(cli.Group.Config().BootstrapAddresses)
			if err != nil {
				return fmt.Errorf("error bootstrapping: %w", err)
			}

			if err = p2pHost.WaitForBootstrap(1+len(cli.Group.Config().Signers)/2, 30*time.Second); err != nil {
				return err
			}

			return nil
		})(c)
		if err != nil {
			return err
		}

		return WithPubsub(pubsubwrapper.WrapLibp2p(p2pHost.GetPubSub()))(c)
	}
}
