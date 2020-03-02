package gossip

import (
	"testing"

	"github.com/ipfs/go-datastore"
	dsync "github.com/ipfs/go-datastore/sync"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundHolderSetCurrent(t *testing.T) {
	rh := newRoundHolder(dsync.MutexWrap(datastore.NewMapDatastore()))

	rh.SetCurrent(newRound(0, 0, 0, 0))
	require.Equal(t, uint64(0), rh.Current().height)

	rh.SetCurrent(newRound(1, 0, 0, 0))
	assert.Equal(t, uint64(1), rh.Current().height)
}

func TestRoundHolderGet(t *testing.T) {
	rh := newRoundHolder(dsync.MutexWrap(datastore.NewMapDatastore()))

	r := newRound(0, 0, 0, 0)
	rh.SetCurrent(r)

	returned, exists := rh.Get(0)

	assert.Equal(t, r, returned)
	assert.True(t, exists)

	_, shouldntExist := rh.Get(1)
	assert.False(t, shouldntExist)
}
