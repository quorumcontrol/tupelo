package gossip

import (
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/quorumcontrol/messages/v2/build/go/services"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/testhelpers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestMempoolContains(t *testing.T) {
	sw := &safewrap.SafeWrap{}

	pool, id, _ := newPopulatedMempool()
	badID := sw.WrapObject("not there").Cid()

	assert.True(t, pool.Contains(id))
	assert.False(t, pool.Contains(badID))
	assert.False(t, pool.Contains(id, badID))
}

func TestMempoolGet(t *testing.T) {
	sw := &safewrap.SafeWrap{}

	pool, id, _ := newPopulatedMempool()
	badID := sw.WrapObject("not there").Cid()

	assert.NotNil(t, pool.Get(id))
	assert.Nil(t, pool.Get(badID))
}

func TestMempoolDeleteIDAndConflictSet(t *testing.T) {
	t.Run("it deletes a single id", func(t *testing.T) {
		pool, id, _ := newPopulatedMempool()

		assert.NotNil(t, pool.Get(id))
		pool.DeleteIDAndConflictSet(id)
		assert.Nil(t, pool.Get(id))
		assert.False(t, pool.Contains(id))
		assert.Equal(t, 0, pool.Length())
	})

	t.Run("it deletes all ids from conflictset as well", func(t *testing.T) {
		treeKey, err := crypto.GenerateKey()
		require.Nil(t, err)

		sw := &safewrap.SafeWrap{}
		abr1 := testhelpers.NewValidTransactionWithPathAndValue(t, treeKey, "/path", "value")
		abr2 := testhelpers.NewValidTransactionWithPathAndValue(t, treeKey, "/path", "differentvalue")
		abr1Cid := sw.WrapObject(abr1).Cid()
		abr2Cid := sw.WrapObject(abr2).Cid()
		t.Logf("abr1: %s, abr2: %s", abr1Cid.String(), abr2Cid.String())

		// abr1 and abr2 are conflicting blocks added to the same chaintree
		require.Equal(t, abr1.ObjectId, abr2.ObjectId)
		require.Equal(t, abr1.Height, abr2.Height)
		require.NotEqual(t, abr1.NewTip, abr2.NewTip)

		pool := newMempool()
		pool.Add(abr1Cid, &abr1)
		pool.Add(abr2Cid, &abr2)
		assert.True(t, pool.Contains(abr1Cid, abr2Cid))
		pool.DeleteIDAndConflictSet(abr2Cid)
		assert.False(t, pool.Contains(abr1Cid))
		assert.False(t, pool.Contains(abr2Cid))
	})
}

func TestMempoolPreferred(t *testing.T) {
	sw := &safewrap.SafeWrap{}
	// use a canonical key because that influences the hash of the ABRs
	keyHex := "0xcd24af4d6c47530202f00442282fa23e06c1adea93e0264cacabf274241918d2"
	treeKey, err := crypto.ToECDSA(hexutil.MustDecode(keyHex))
	require.Nil(t, err)

	abr1 := testhelpers.NewValidTransactionWithPathAndValue(t, treeKey, "/path", "value")
	abr2 := testhelpers.NewValidTransactionWithPathAndValue(t, treeKey, "/path", "differentvalue")
	abr1Cid := sw.WrapObject(abr1).Cid()
	abr2Cid := sw.WrapObject(abr2).Cid()
	t.Logf("abr1: %s, abr2: %s", abr1Cid.String(), abr2Cid.String())

	// abr1 and abr2 are conflicting blocks added to the same chaintree
	require.Equal(t, abr1.ObjectId, abr2.ObjectId)
	require.Equal(t, abr1.Height, abr2.Height)
	require.NotEqual(t, abr1.NewTip, abr2.NewTip)
	// abr2 is a better abr because it has a lower hash value
	require.True(t, abr2Cid.String() < abr1Cid.String())

	pool := newMempool()
	pool.Add(abr1Cid, &abr1)
	require.True(t, preferredContains(pool, abr1Cid))

	pool.Add(abr2Cid, &abr2)
	assert.True(t, preferredContains(pool, abr2Cid))
	assert.False(t, preferredContains(pool, abr1Cid))
}

func preferredContains(pool *mempool, id cid.Cid) bool {
	for _, pID := range pool.Preferred() {
		if pID.Equals(id) {
			return true
		}
	}
	return false
}
