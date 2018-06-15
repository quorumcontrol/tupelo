package consensus

import (
	"fmt"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
)

type History interface {
	StoreBlocks([]*consensuspb.Block) error
	GetBlock(hsh []byte) *consensuspb.Block
	NextBlock(hsh []byte) *consensuspb.Block
	Length() uint64
	Blocks() []*consensuspb.Block
	PrevBlock(hsh []byte) *consensuspb.Block
	IteratorFrom(block *consensuspb.Block, transaction *consensuspb.Transaction) TransactionIterator
}

type TransactionIterator interface {
	Next() TransactionIterator
	Prev() TransactionIterator
	Transaction() *consensuspb.Transaction
	Block() *consensuspb.Block
}

type memoryTransactionIterator struct {
	blockHash        []byte
	transactionIndex int
	history          History
	transaction      *consensuspb.Transaction
}

type memoryHistoryStore struct {
	blocks         map[string]*consensuspb.Block
	forwardMapping map[string]string
}

// just make sure it implements the interface
var _ History = (*memoryHistoryStore)(nil)

func NewMemoryHistoryStore() *memoryHistoryStore {
	return &memoryHistoryStore{
		blocks:         make(map[string]*consensuspb.Block),
		forwardMapping: make(map[string]string),
	}
}

func (h *memoryHistoryStore) Blocks() []*consensuspb.Block {
	blocks := make([]*consensuspb.Block, len(h.blocks))
	i := 0
	for _, block := range h.blocks {
		blocks[i] = block
		i++
	}
	return blocks
}

func (h *memoryHistoryStore) GetBlock(hsh []byte) *consensuspb.Block {
	block, ok := h.blocks[hexutil.Encode(hsh)]
	if ok {
		return block
	} else {
		return nil
	}
}

func (h *memoryHistoryStore) NextBlock(hsh []byte) *consensuspb.Block {
	forward, ok := h.forwardMapping[hexutil.Encode(hsh)]
	if ok {
		return h.blocks[forward]
	} else {
		return nil
	}
}

func (h *memoryHistoryStore) PrevBlock(hsh []byte) *consensuspb.Block {
	current := h.GetBlock(hsh)
	if current != nil {
		return h.GetBlock(current.SignableBlock.PreviousHash)
	}

	return nil
}

func (h *memoryHistoryStore) Length() uint64 {
	return uint64(len(h.blocks))
}

func (h *memoryHistoryStore) StoreBlocks(blocks []*consensuspb.Block) error {
	for _, block := range blocks {
		hsh, err := BlockToHash(block)
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

func (h *memoryHistoryStore) IteratorFrom(block *consensuspb.Block, transaction *consensuspb.Transaction) TransactionIterator {
	index := indexOfTransaction(block.SignableBlock.Transactions, transaction)
	if index > -1 {
		return &memoryTransactionIterator{
			history:          h,
			transaction:      transaction,
			blockHash:        MustBlockToHash(block).Bytes(),
			transactionIndex: index,
		}
	}
	return nil
}

func indexOfTransaction(transactions []*consensuspb.Transaction, transaction *consensuspb.Transaction) int {
	for i, t := range transactions {
		if t == transaction {
			return i
		}
	}
	return -1
}

func (ti *memoryTransactionIterator) Next() TransactionIterator {
	block := ti.history.GetBlock(ti.blockHash)
	newIndex := ti.transactionIndex + 1
	var transaction *consensuspb.Transaction

	if newIndex < len(block.SignableBlock.Transactions) {
		transaction = block.SignableBlock.Transactions[newIndex]
	} else {
		block = ti.history.NextBlock(ti.blockHash)
		newIndex = 0
	}

	if block != nil {
		transaction = block.SignableBlock.Transactions[newIndex]

		return &memoryTransactionIterator{
			transaction:      transaction,
			transactionIndex: newIndex,
			blockHash:        MustBlockToHash(block).Bytes(),
			history:          ti.history,
		}
	}

	return nil
}

func (ti *memoryTransactionIterator) Prev() TransactionIterator {
	block := ti.history.GetBlock(ti.blockHash)
	newIndex := ti.transactionIndex - 1
	var transaction *consensuspb.Transaction

	if newIndex >= 0 {
		log.Trace("new index from iterator", "index", newIndex)
		transaction = block.SignableBlock.Transactions[newIndex]
	} else {
		block = ti.history.PrevBlock(ti.blockHash)
		if block != nil {
			newIndex = len(block.SignableBlock.Transactions) - 1
		}
	}

	if block != nil {
		transaction = block.SignableBlock.Transactions[newIndex]

		return &memoryTransactionIterator{
			transaction:      transaction,
			transactionIndex: newIndex,
			blockHash:        MustBlockToHash(block).Bytes(),
			history:          ti.history,
		}
	}

	log.Trace("return nil from iterator")
	return nil
}

func (ti *memoryTransactionIterator) Transaction() *consensuspb.Transaction {
	return ti.transaction
}

func (ti *memoryTransactionIterator) Block() *consensuspb.Block {
	return ti.history.GetBlock(ti.blockHash)
}
