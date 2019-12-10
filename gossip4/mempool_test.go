package gossip4

import (
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/stretchr/testify/assert"
)

func newPopulatedMempool() (*mempool, cid.Cid, *services.AddBlockRequest) {
	sw := &safewrap.SafeWrap{}
	abr := &services.AddBlockRequest{
		ObjectId: []byte("test"),
	}
	n := sw.WrapObject(abr)

	pool := newMempool()
	pool.Add(n.Cid(), abr)
	return pool, n.Cid(), abr
}

func TestMempoolKeys(t *testing.T) {
	pool, id, _ := newPopulatedMempool()
	keys := pool.Keys()
	assert.Len(t, keys, 1)
	assert.True(t, keys[0].Equals(id))
}

func TestMempoolContains(t *testing.T) {
	sw := &safewrap.SafeWrap{}

	pool, id, _ := newPopulatedMempool()
	badID := sw.WrapObject("not there").Cid()

	assert.True(t, pool.Contains(id))
	assert.False(t, pool.Contains(badID))
	assert.False(t, pool.Contains(id, badID))
}
