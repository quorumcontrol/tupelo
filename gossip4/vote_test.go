package gossip4

import (
	"crypto/rand"
	"math"
	"testing"

	"github.com/ipfs/go-cid"
	logging "github.com/ipfs/go-log"
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
			Block: nil,
		}
		v.Nil()
		votes = append(votes, v)

		expectedTallies[ZeroVoteID] = .32
	}

	// Vote 2: has the least non-zero transactions, we expect it to be zero.
	{
		nodeIDCount++

		block := newBlock(1, generateTxIds(t, 2))

		votes = append(votes, &Vote{
			Block: block,
		})

		expectedTallies[block.ID()] = 0.0
	}

	// Vote 3: has the highest transactions.
	{
		nodeIDCount++
		block := newBlock(1, generateTxIds(t, 10))

		votes = append(votes, &Vote{
			Block: block,
		})

		expectedTallies[block.ID()] = 0.32
	}

	// Vote 4: has 4 txs
	{
		nodeIDCount++
		block := newBlock(1, generateTxIds(t, 4))

		votes = append(votes, &Vote{
			Block: block,
		})
		expectedTallies[block.ID()] = 0.08
	}

	// Vote 5: Second highest transactions.
	{
		nodeIDCount++
		block := newBlock(1, generateTxIds(t, 9))
		votes = append(votes, &Vote{
			Block: block,
		})

		expectedTallies[block.ID()] = 0.279999999999999
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

	logging.SetLogLevel("snowball", "debug")
	t.Run("no decision in case of all equal tallies", func(t *testing.T) {
		snowball.Reset()

		votes := make([]*Vote, snowball.k)

		for i := 0; i < len(votes); i++ {
			block := newBlock(1, generateTxIds(t, 2))
			votes[i] = &Vote{
				Block: block,
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

		_block := newBlock(1, generateTxIds(t, 1))

		for i := 0; i < cap(votes); i++ {
			var block *Block
			if i < int(snowball.alpha*float64(cap(votes))) {
				block = _block
			}

			votes = append(votes, &Vote{
				Block: block,
			})
		}

		for i := 0; i < snowball.beta+1; i++ {
			snowball.Tick(calculateTallies(votes))
		}

		assert.False(t, snowball.Decided())
		assert.Equal(t, _block.ID(), snowball.Preferred().Block.ID())
	})

	t.Run("transactions num majority block wins", func(t *testing.T) {
		snowball.Reset()

		biggestTxNumIdx := 0
		votes := make([]*Vote, 0, snowball.k)

		for i := 0; i < cap(votes); i++ {
			num := 2
			if i == biggestTxNumIdx {
				num++
			}

			block := newBlock(1, generateTxIds(t, num))

			votes = append(votes, &Vote{
				Block: block,
			})
		}

		for i := 0; i < snowball.beta+1; i++ {
			snowball.Tick(calculateTallies(votes))
		}

		assert.True(t, snowball.Decided())
		assert.Equal(t, votes[biggestTxNumIdx].Block.ID(), snowball.Preferred().Block.ID())
	})
}

// func TestCollectVotesForSync(t *testing.T) {
// 	snowballK := 10
// 	defaultBeta := conf.GetSnowballBeta()
// 	conf.Update(conf.WithSnowballBeta(5))
// 	defer func() {
// 		conf.Update(conf.WithSnowballBeta(defaultBeta))
// 	}()

// 	snowball := NewSnowball()

// 	t.Run("success - decision made", func(t *testing.T) {
// 		accounts := NewAccounts(store.NewInmem())
// 		snowball.Reset()

// 		votes := make([]Vote, 0, snowballK)
// 		voteC := make(chan Vote)
// 		wg := new(sync.WaitGroup)

// 		wg.Add(1)
// 		go CollectVotesForSync(accounts, snowball, voteC, wg, snowballK)

// 		for i := 0; i < cap(votes); i++ {
// 			votes = append(votes, &syncVote{
// 				voter:     getRandomID(t),
// 				outOfSync: true,
// 			})
// 		}

// 		for i := 0; i < conf.GetSnowballBeta()+1; i++ {
// 			for n := range votes {
// 				voteC <- votes[n]
// 			}
// 		}

// 		close(voteC)
// 		wg.Wait()

// 		assert.True(t, snowball.Decided())
// 		assert.True(t, *snowball.preferred.Value().(*bool))
// 	})

// 	t.Run("success - one of the voters votes wrong, but majority is enough", func(t *testing.T) {
// 		accounts := NewAccounts(store.NewInmem())
// 		snowball.Reset()

// 		votes := make([]Vote, 0, snowballK)
// 		voteC := make(chan Vote)
// 		wg := new(sync.WaitGroup)

// 		wg.Add(1)
// 		go CollectVotesForSync(accounts, snowball, voteC, wg, snowballK)

// 		for i := 0; i < cap(votes); i++ {
// 			outOfSync := true
// 			if i == 0 {
// 				outOfSync = false
// 			}

// 			votes = append(votes, &syncVote{
// 				voter:     getRandomID(t),
// 				outOfSync: outOfSync,
// 			})
// 		}

// 		for i := 0; i < conf.GetSnowballBeta()+1; i++ {
// 			for n := range votes {
// 				voteC <- votes[n]
// 			}
// 		}

// 		close(voteC)
// 		wg.Wait()

// 		assert.True(t, snowball.Decided())
// 		assert.True(t, *snowball.preferred.Value().(*bool))
// 	})

// 	t.Run("success - 50-50 votes, but one vote has the highest stake", func(t *testing.T) {
// 		accounts := NewAccounts(store.NewInmem())
// 		snapshot := accounts.Snapshot()
// 		snowball.Reset()

// 		votes := make([]Vote, 0, snowballK)
// 		voteC := make(chan Vote)
// 		wg := new(sync.WaitGroup)

// 		wg.Add(1)
// 		go CollectVotesForSync(accounts, snowball, voteC, wg, snowballK)

// 		for i := 0; i < cap(votes); i++ {
// 			voter := getRandomID(t)

// 			outOfSync := false
// 			if i%2 == 0 {
// 				outOfSync = true
// 			}

// 			stake := sys.MinimumStake
// 			if i == 0 {
// 				stake *= 10
// 			}
// 			WriteAccountStake(snapshot, voter.PublicKey(), stake)

// 			votes = append(votes, &syncVote{
// 				voter:     voter,
// 				outOfSync: outOfSync,
// 			})
// 		}

// 		assert.NoError(t, accounts.Commit(snapshot))

// 		for i := 0; i < conf.GetSnowballBeta()+1; i++ {
// 			for n := range votes {
// 				voteC <- votes[n]
// 			}
// 		}

// 		close(voteC)
// 		wg.Wait()

// 		assert.True(t, snowball.Decided())
// 		assert.True(t, *snowball.Preferred().Value().(*bool))
// 	})

// 	t.Run("no decision - 50-50 votes", func(t *testing.T) {
// 		accounts := NewAccounts(store.NewInmem())
// 		snowball.Reset()

// 		votes := make([]Vote, 0, snowballK)
// 		voteC := make(chan Vote)
// 		wg := new(sync.WaitGroup)

// 		wg.Add(1)
// 		go CollectVotesForSync(accounts, snowball, voteC, wg, snowballK)

// 		for i := 0; i < cap(votes); i++ {
// 			voter := getRandomID(t)

// 			outOfSync := true
// 			if i%2 == 0 {
// 				outOfSync = false
// 			}

// 			votes = append(votes, &syncVote{
// 				voter:     voter,
// 				outOfSync: outOfSync,
// 			})
// 		}

// 		for i := 0; i < conf.GetSnowballBeta()+1; i++ {
// 			for n := range votes {
// 				voteC <- votes[n]
// 			}
// 		}

// 		close(voteC)
// 		wg.Wait()

// 		assert.False(t, snowball.Decided())
// 	})

// 	t.Run("no decision - less than snowballK voter", func(t *testing.T) {
// 		accounts := NewAccounts(store.NewInmem())
// 		snowball.Reset()

// 		votes := make([]Vote, 0, snowballK)
// 		voteC := make(chan Vote)
// 		wg := new(sync.WaitGroup)

// 		wg.Add(1)
// 		go CollectVotesForSync(accounts, snowball, voteC, wg, snowballK)

// 		for i := 0; i < cap(votes)-1; i++ {
// 			votes = append(votes, &syncVote{
// 				voter:     getRandomID(t),
// 				outOfSync: true,
// 			})
// 		}

// 		for i := 0; i < conf.GetSnowballBeta()+1; i++ {
// 			for n := range votes {
// 				voteC <- votes[n]
// 			}
// 		}

// 		close(voteC)
// 		wg.Wait()

// 		assert.False(t, snowball.Decided())
// 	})
// }
