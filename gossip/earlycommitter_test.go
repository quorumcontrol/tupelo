package gossip

import (
	"testing"

	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEarlyCommit(t *testing.T) {
	sw := &safewrap.SafeWrap{}

	c1 := sw.WrapObject("1").Cid()
	c2 := sw.WrapObject("2").Cid()
	c3 := sw.WrapObject("3").Cid()

	require.Nil(t, sw.Err)

	ec := newEarlyCommitter()

	ec.Vote("1", c1)
	ec.Vote("2", c2)
	ec.Vote("3", c3)
	hasThreshold, _ := ec.HasThreshold(3, 0.66)
	require.False(t, hasThreshold)

	ec.Vote("2", c1)
	hasThreshold, checkpoint := ec.HasThreshold(3, 0.66)

	assert.True(t, hasThreshold)
	assert.Truef(t, c1.Equals(checkpoint), "expected %s to equal %s", checkpoint.String(), c1.String())
}
