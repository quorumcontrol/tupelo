package gossip

import (
	"context"
	"testing"

	"github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/tupelo/sdk/gossip/hamtwrapper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundHolderSetCurrent(t *testing.T) {
	ctx := context.Background()
	dataStore := dsync.MutexWrap(datastore.NewMapDatastore())
	dagStore := nodestore.MustMemoryStore(ctx)
	hamtStore := hamtwrapper.DagStoreToCborIpld(dagStore)

	rhOpts := &roundHolderOpts{
		DataStore: dataStore,
		DagStore:  dagStore,
		HamtStore: hamtStore,
	}
	rh, err := newRoundHolder(rhOpts)
	require.Nil(t, err)

	rh.SetCurrent(newRound(0, 0, 0, 0))
	require.Equal(t, uint64(0), rh.Current().height)

	rh.SetCurrent(newRound(1, 0, 0, 0))
	assert.Equal(t, uint64(1), rh.Current().height)
}

func TestRoundHolderGet(t *testing.T) {
	ctx := context.Background()
	dataStore := dsync.MutexWrap(datastore.NewMapDatastore())
	dagStore := nodestore.MustMemoryStore(ctx)
	hamtStore := hamtwrapper.DagStoreToCborIpld(dagStore)

	rhOpts := &roundHolderOpts{
		DataStore: dataStore,
		DagStore:  dagStore,
		HamtStore: hamtStore,
	}
	rh, err := newRoundHolder(rhOpts)
	require.Nil(t, err)

	r := newRound(0, 0, 0, 0)
	rh.SetCurrent(r)

	returned, exists := rh.Get(0)

	assert.Equal(t, r, returned)
	assert.True(t, exists)

	_, shouldntExist := rh.Get(1)
	assert.False(t, shouldntExist)
}
