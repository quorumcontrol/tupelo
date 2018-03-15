package consensus

import (
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

type History interface {
	StoreBlocks([]*consensuspb.Block) error
	GetBlock(hsh []byte) (*consensuspb.Block)
	NextBlock(hsh []byte) (*consensuspb.Block)
	Length() (uint64)
}

type memoryHistoryStore struct {
	blocks map[string]*consensuspb.Block
	forwardMapping map[string]string
}

// just make sure it implements the interface
var _ History = (*memoryHistoryStore)(nil)

func NewMemoryHistoryStore() *memoryHistoryStore {
	return &memoryHistoryStore{
		blocks: make(map[string]*consensuspb.Block),
		forwardMapping: make(map[string]string),
	}
}

func (h *memoryHistoryStore) GetBlock(hsh []byte) *consensuspb.Block {
	block,ok := h.blocks[hexutil.Encode(hsh)]
	if ok {
		return block
	} else {
		return nil
	}
}

func (h *memoryHistoryStore) NextBlock(hsh []byte) *consensuspb.Block {
	forward,ok := h.forwardMapping[hexutil.Encode(hsh)]
	if ok {
		return h.blocks[forward]
	} else {
		return nil
	}
}

func (h *memoryHistoryStore) Length() (uint64) {
	return uint64(len(h.blocks))
}

func (h *memoryHistoryStore) StoreBlocks(blocks []*consensuspb.Block) error {
	for _,block := range blocks {
		hsh,err := BlockToHash(block)
		if err != nil {
			return fmt.Errorf("error hashing block: %v", err)
		}
		if block.SignableBlock.PreviousHash != nil {
			h.forwardMapping[hexutil.Encode(block.SignableBlock.PreviousHash)] = hexutil.Encode(hsh.Bytes())
		}
		h.blocks[hexutil.Encode(hsh.Bytes())] = block
	}
	return nil
}