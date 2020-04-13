package nodebuilder

import (
	"context"
	"testing"

	"github.com/quorumcontrol/tupelo/sdk/p2p"

	"github.com/quorumcontrol/tupelo/sdk/gossip/types"

	"github.com/quorumcontrol/tupelo/server/testnotarygroup"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
)

func TestBootstrap(t *testing.T) {

	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	t.Run("with basic config", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		nb := &NodeBuilder{
			Config: &Config{
				PrivateKeySet: &PrivateKeySet{
					DestKey: key,
				},
				BootstrapNodes: p2p.BootstrapNodes(),
				BootstrapOnly:  true,
				NotaryGroupConfig: &types.Config{
					TransactionTopic: "testOnly",
				},
			},
		}
		err = nb.Start(ctx)
		require.Nil(t, err)
	})
}

func TestSigner(t *testing.T) {
	ts := testnotarygroup.NewTestSet(t, 3)

	t.Run("with basic config", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		bootstrapHost, err := p2p.NewHostFromOptions(ctx)
		require.Nil(t, err)

		addrs := testnotarygroup.BootstrapAddresses(bootstrapHost)

		ngConfig := types.DefaultConfig()
		ngConfig.ID = "hardcoded"
		ngConfig.Signers = []types.PublicKeySet{
			types.PublicKeySet{
				DestKey: &ts.EcdsaKeys[1].PublicKey,
				VerKey:  ts.SignKeys[1].MustVerKey(),
			},
			types.PublicKeySet{
				DestKey: &ts.EcdsaKeys[2].PublicKey,
				VerKey:  ts.SignKeys[2].MustVerKey(),
			},
		}

		hsc := &HumanStorageConfig{Kind: "memory"}
		bstore, err := hsc.ToBlockstore()
		require.Nil(t, err)

		dstore, err := hsc.ToDatastore()
		require.Nil(t, err)

		nb := &NodeBuilder{
			Config: &Config{
				NotaryGroupConfig: ngConfig,
				PrivateKeySet: &PrivateKeySet{
					DestKey: ts.EcdsaKeys[0],
					SignKey: ts.SignKeys[0],
				},
				Blockstore:     bstore,
				Datastore:      dstore,
				BootstrapNodes: addrs,
			},
		}
		err = nb.Start(ctx)
		require.Nil(t, err)
		err = nb.Stop()
		require.Nil(t, err)
	})
}
