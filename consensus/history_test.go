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