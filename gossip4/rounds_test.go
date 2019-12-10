package gossip4

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRoundHolderSetCurrent(t *testing.T) {
	rh := newRoundHolder()

	rh.SetCurrent(newRound(0))
	require.Equal(t, uint64(0), rh.Current().height)

	rh.SetCurrent(newRound(1))
	assert.Equal(t, uint64(1), rh.Current().height)
}

func TestRoundHolderGet(t *testing.T) {
	rh := newRoundHolder()

	r := newRound(0)
	rh.SetCurrent(r)

	returned, exists := rh.Get(0)

	assert.Equal(t, r, returned)
	assert.True(t, exists)

	_, shouldntExist := rh.Get(1)
	assert.False(t, shouldntExist)
}
