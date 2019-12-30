package gossip4

import (
	"crypto/rand"
	"math"
	"testing"

	"github.com/ipfs/go-cid"
	"github.com/quorumcontrol/chaintree/safewrap"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func generateTxIds(t *testing.T, count int) []cid.Cid {
	sw := &safewrap.SafeWrap{}
	var ids []cid.Cid

	for i := 0; i < count; i++ {
		randomBits := make([]byte, 32)
		rand.Read(randomBits)

		n := sw.WrapObject(randomBits)
		require.NoError(t, sw.Err)

		ids = append(ids, n.Cid())
	}

	return ids
}

func TestVoteWithoutBlock(t *testing.T) {
	v := &Vote{}

	assert.Equal(t, ZeroVoteID, v.ID())
	assert.Zero(t, v.Length())
}

func TestCalculateTallies(t *testing.T) {
	var votes []*Vote
	expectedTallies := make(map[string]float64)

	nodeIDCount := 0

	// Vote 1: empty vote
	{
		nodeIDCount++
		v := &Vote{
			Checkpoint: nil,
		}
		v.Nil()
		votes = append(votes, v)

		expectedTallies[ZeroVoteID] = .32
	}

	// Vote 2: has the least non-zero transactions, we expect it to be zero.
	{
		nodeIDCount++

		checkpoint := newCheckpoint(1, generateTxIds(t, 2))

		votes = append(votes, &Vote{
			Checkpoint: checkpoint,
		})

		expectedTallies[checkpoint.ID()] = 0.0
	}

	// Vote 3: has the highest transactions.
	{
		nodeIDCount++
		checkpoint := newCheckpoint(1, generateTxIds(t, 10))

		votes = append(votes, &Vote{
			Checkpoint: checkpoint,
		})

		expectedTallies[checkpoint.ID()] = 0.32
	}

	// Vote 4: has 4 txs
	{
		nodeIDCount++
		checkpoint := newCheckpoint(1, generateTxIds(t, 4))

		votes = append(votes, &Vote{
			Checkpoint: checkpoint,
		})
		expectedTallies[checkpoint.ID()] = 0.08
	}

	// Vote 5: Second highest transactions.
	{
		nodeIDCount++
		checkpoint := newCheckpoint(1, generateTxIds(t, 9))
		votes = append(votes, &Vote{
			Checkpoint: checkpoint,
		})

		expectedTallies[checkpoint.ID()] = 0.279999999999999
	}

	tallies := calculateTallies(votes)
	assert.Len(t, tallies, nodeIDCount)

	// Because of floating point inaccuracy, we convert it to an integer.
	toInt64 := func(v float64) int64 {
		// Use 10 ^ 15 because it seems the inaccuracy is happening at 10 ^ -16.
		exp := math.Pow10(15)
		return int64(exp * v)
	}

	for _, vote := range tallies {
		assert.Equal(t, toInt64(expectedTallies[vote.ID()]), toInt64(vote.Tally()))
	}
}

func TestTick(t *testing.T) {

	snowball := NewSnowball(defaultAlpha, defaultBeta, defaultK)

	t.Run("no decision in case of all equal tallies", func(t *testing.T) {
		snowball.Reset()

		votes := make([]*Vote, snowball.k)

		for i := 0; i < len(votes); i++ {
			checkpoint := newCheckpoint(1, generateTxIds(t, 2))
			votes[i] = &Vote{
				Checkpoint: checkpoint,
			}
		}

		for i := 0; i < snowball.beta+1; i++ {
			snowball.Tick(calculateTallies(votes))
		}

		assert.False(t, snowball.Decided())
		assert.Nil(t, snowball.Preferred())
	})

	t.Run("no decision in case of 20% empty votes", func(t *testing.T) {
		snowball.Reset()

		votes := make([]*Vote, 0, snowball.k)

		_checkpoint := newCheckpoint(1, generateTxIds(t, 1))

		for i := 0; i < cap(votes); i++ {
			var checkpoint *Checkpoint
			if i < int(snowball.alpha*float64(cap(votes))) {
				checkpoint = _checkpoint
			}

			votes = append(votes, &Vote{
				Checkpoint: checkpoint,
			})
		}

		for i := 0; i < snowball.beta+1; i++ {
			snowball.Tick(calculateTallies(votes))
		}

		assert.False(t, snowball.Decided())
		assert.Equal(t, _checkpoint.ID(), snowball.Preferred().Checkpoint.ID())
	})

	t.Run("transactions num majority checkpoint wins", func(t *testing.T) {
		snowball.Reset()

		biggestTxNumIdx := 0
		votes := make([]*Vote, 0, snowball.k)

		for i := 0; i < cap(votes); i++ {
			num := 2
			if i == biggestTxNumIdx {
				num++
			}

			checkpoint := newCheckpoint(1, generateTxIds(t, num))

			votes = append(votes, &Vote{
				Checkpoint: checkpoint,
			})
		}

		for i := 0; i < snowball.beta+1; i++ {
			snowball.Tick(calculateTallies(votes))
		}

		assert.True(t, snowball.Decided())
		assert.Equal(t, votes[biggestTxNumIdx].Checkpoint.ID(), snowball.Preferred().Checkpoint.ID())
	})
}
