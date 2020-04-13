package client

import (
	"bytes"
	"context"
	"testing"

	datastore "github.com/ipfs/go-datastore"
	dsq "github.com/ipfs/go-datastore/query"
	dsync "github.com/ipfs/go-datastore/sync"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/dag"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/transactions"
	"github.com/quorumcontrol/tupelo/sdk/consensus"
	"github.com/quorumcontrol/tupelo/sdk/gossip/client/pubsubinterfaces/pubsubwrapper"
	"github.com/quorumcontrol/tupelo/sdk/gossip/types"
	"github.com/stretchr/testify/require"
)

func TestWithP2pOpts(t *testing.T) {
	pubsub := pubsubwrapper.WrapLibp2p(nil)
	dagstore := nodestore.MustMemoryStore(context.Background())
	cli := New(WithPubsub(pubsub), WithDagStore(dagstore))
	require.Equal(t, pubsub, cli.pubsub)
	require.Equal(t, dagstore, cli.DagStore())
}

func TestWithValidators(t *testing.T) {
	validators := []chaintree.BlockValidatorFunc{
		func(_ *dag.Dag, _ *chaintree.BlockWithHeaders) (bool, chaintree.CodedError) {
			return true, nil
		},
	}
	cli := New(WithValidators(validators))
	require.Equal(t, validators, cli.validators)
}

func TestWithTransactors(t *testing.T) {
	transactors := map[transactions.Transaction_Type]chaintree.TransactorFunc{
		transactions.Transaction_SETDATA: consensus.SetDataTransaction,
	}
	cli := New(WithTransactors(transactors))
	require.Equal(t, transactors, cli.transactors)
}

func TestWithStorage(t *testing.T) {
	storage := dsync.MutexWrap(datastore.NewMapDatastore())
	sw := &safewrap.SafeWrap{}
	data := sw.WrapObject("test123")
	require.Nil(t, sw.Err)

	cli := New(WithStorage(storage))
	err := cli.DagStore().Add(context.Background(), data)
	require.Nil(t, err)

	results, err := storage.Query(dsq.Query{})
	require.Nil(t, err)

	entries, err := results.Rest()
	require.Nil(t, err)

	var found bool
	for _, e := range entries {
		if bytes.Equal(e.Value, data.RawData()) {
			found = true
			continue
		}
	}
	require.True(t, found)
}

func TestWithNotaryGroupToml(t *testing.T) {
	cli := New(WithNotaryGroupToml(notaryGroupLocalDocker))
	require.Equal(t, cli.Group.ID, "tupelolocal")
}

func TestWithNamedNotaryGroup(t *testing.T) {
	cli := New(WithNamedNotaryGroup("tupelolocal"))
	require.Equal(t, cli.Group.ID, "tupelolocal")
}

func TestWithNotaryGroup(t *testing.T) {
	ngConfig, err := types.TomlToConfig(notaryGroupLocalDocker)
	require.Nil(t, err)
	ng, err := ngConfig.NotaryGroup(nil)
	require.Nil(t, err)

	validators, err := ng.BlockValidators(context.Background())
	require.True(t, len(validators) > 0)
	require.Nil(t, err)
	transactors := ngConfig.Transactions
	require.True(t, len(transactors) > 0)

	cli := New(WithNotaryGroup(ng))
	require.Len(t, cli.validators, len(validators)+1)
	require.Equal(t, transactors, cli.transactors)
}
