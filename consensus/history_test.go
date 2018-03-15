package consensus_test

import (
	"github.com/quorumcontrol/qc3/consensus"
	"testing"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"github.com/stretchr/testify/assert"
)

func TestMemoryHistoryStore(t *testing.T) {
	history := consensus.NewMemoryHistoryStore()

	blocks := make([]*consensuspb.Block, 3)

	for i:=0;i<len(blocks);i++ {
		blocks[i] = createBlock(t, nil)
	}

	history.StoreBlocks(blocks)

	for _,block := range blocks {
		hsh,err := consensus.BlockToHash(block)
		assert.Nil(t,err)
		retBlock := history.GetBlock(hsh.Bytes())
		assert.Equal(t, block,retBlock)
	}
}

func TestMemoryHistoryStore_NextBlock(t *testing.T) {
	history := consensus.NewMemoryHistoryStore()

	blocks := make([]*consensuspb.Block, 4)

	blocks[0] = createBlock(t, nil)

	for i := 1; i < len(blocks); i++ {
		blocks[i] = createBlock(t, blocks[i-1])
	}

	history.StoreBlocks(blocks)

	for i := 0; i<len(blocks)-1; i++ {
		hsh,err := consensus.BlockToHash(blocks[i])
		assert.Nil(t,err)
		blk := history.NextBlock(hsh.Bytes())
		assert.Equal(t, blk, blocks[i+1], "failed on block %d", i)
	}
}

func TestMemoryHistoryStore_PrevBlock(t *testing.T) {
	history := consensus.NewMemoryHistoryStore()

	blocks := make([]*consensuspb.Block, 4)

	blocks[0] = createBlock(t, nil)

	for i := 1; i < len(blocks); i++ {
		blocks[i] = createBlock(t, blocks[i-1])
	}

	history.StoreBlocks(blocks)

	for i := len(blocks)-1; i > 0; i-- {
		hsh,err := consensus.BlockToHash(blocks[i])
		assert.Nil(t,err)
		blk := history.PrevBlock(hsh.Bytes())
		assert.Equal(t, blk, blocks[i-1], "failed on block %d", i)
	}
}

func TestMemoryHistoryStore_IteratorFrom(t *testing.T) {
	history := consensus.NewMemoryHistoryStore()

	blocks := make([]*consensuspb.Block, 4)

	blocks[0] = createBlock(t, nil)

	for i := 1; i < len(blocks); i++ {
		blocks[i] = createBlock(t, blocks[i-1])
	}

	history.StoreBlocks(blocks)

	for _,test := range []struct{
		description string
		block *consensuspb.Block
		transaction *consensuspb.Transaction
		shouldBeNil bool
	} {
		{
			description: "a valid iterator",
			block: blocks[1],
			transaction: blocks[1].SignableBlock.Transactions[0],
			shouldBeNil: false,
		},{
			description: "an iterator with an unknown transaction should be nil",
			block: blocks[1],
			transaction: &consensuspb.Transaction{},
			shouldBeNil: true,
		},{
			description: "an iterator with a nil transaction should be nil",
			block: blocks[1],
			transaction: nil,
			shouldBeNil: true,
		},
	} {
		iterator := history.IteratorFrom(test.block, test.transaction)
		if test.shouldBeNil {
			assert.Nil(t, iterator, test.description)
		} else {
			assert.NotNil(t, iterator, test.description)
		}
	}
}

func TestMemoryTransactionIterator_Next(t *testing.T) {
	history := consensus.NewMemoryHistoryStore()

	blocks := make([]*consensuspb.Block, 4)

	blocks[0] = createBlock(t, nil)

	for i := 1; i < len(blocks); i++ {
		blocks[i] = createBlock(t, blocks[i-1])
	}

	expectedTransactions := make([]*consensuspb.Transaction, len(blocks))
	for i := 0; i < len(blocks); i++ {
		expectedTransactions[i] = blocks[i].SignableBlock.Transactions[0]
	}

	history.StoreBlocks(blocks)

	retTransactions := make([]*consensuspb.Transaction, len(expectedTransactions))

	iterator := history.IteratorFrom(blocks[0], blocks[0].SignableBlock.Transactions[0])
	i := 0
	for iterator != nil {
		retTransactions[i] = iterator.Transaction()
		iterator = iterator.Next()
		i++
	}

	for i,trans := range retTransactions {
		assert.Equal(t, trans.Id, expectedTransactions[i].Id, "failed equal at: %d", i)
	}
}

func TestMemoryTransactionIterator_Prev(t *testing.T) {
	history := consensus.NewMemoryHistoryStore()

	blocks := make([]*consensuspb.Block, 4)

	blocks[0] = createBlock(t, nil)

	for i := 1; i < len(blocks); i++ {
		blocks[i] = createBlock(t, blocks[i-1])
	}

	expectedTransactions := make([]*consensuspb.Transaction, len(blocks))
	y := 0
	for i := len(blocks) - 1; i >= 0; i-- {
		expectedTransactions[y] = blocks[i].SignableBlock.Transactions[0]
		y++
	}

	history.StoreBlocks(blocks)

	retTransactions := make([]*consensuspb.Transaction, len(expectedTransactions))

	iterator := history.IteratorFrom(blocks[len(blocks)-1], blocks[len(blocks)-1].SignableBlock.Transactions[0])
	i := 0
	for iterator != nil {
		retTransactions[i] = iterator.Transaction()
		iterator = iterator.Prev()
		i++
	}

	for i,trans := range retTransactions {
		assert.Equal(t, trans.Id, expectedTransactions[i].Id, "failed equal at: %d", i)
	}
}