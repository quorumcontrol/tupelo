package notary_test

import (
	"testing"
	"github.com/quorumcontrol/qc3/notary"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/quorumcontrol/qc3/internalchain"
	"github.com/ethereum/go-ethereum/log"
	"os"
)

func TestMintCoinTransactor(t *testing.T) {
	signer := defaultNotary(t)
	defaultChain := chainFromEcdsaKey(t, &aliceKey.PublicKey)

	for _,test := range []struct {
		description string
		transaction *consensuspb.Transaction
		shouldError bool
		shouldInterrupt bool
		shouldSign bool
	} {
		{
			description: "a valid mint",
			transaction: consensus.EncapsulateTransaction(consensuspb.MINT_COIN, &consensuspb.MintCoinTransaction{
				Name: defaultChain.Id + "catCoin",
				Amount: 1000000000, // 1 BILLION cat cat coin
			}),
			shouldError: false,
			shouldInterrupt: false,
			shouldSign: true,
		},
		{
			description: "a mint with an invalid name",
			transaction: consensus.EncapsulateTransaction(consensuspb.MINT_COIN, &consensuspb.MintCoinTransaction{
				Name: "did:someotherchain:"+ "catCoin",
				Amount: 1000000000, // 1 BILLION cat cat coin
			}),
			shouldError: false,
			shouldInterrupt: true,
			shouldSign: false,
		},
	} {
		storage := internalchain.NewMemStorage()
		chain := chainFromEcdsaKey(t, &aliceKey.PublicKey)

		block := &consensuspb.Block{
			SignableBlock: &consensuspb.SignableBlock{
				Sequence: 0,
				ChainId: chain.Id,
				Transactions: []*consensuspb.Transaction{
					test.transaction,
				},
			},
		}
		chain.Blocks = []*consensuspb.Block{block}
		history := consensus.NewMemoryHistoryStore()

		chainTip,err := storage.Get(chain.Id)
		assert.Nil(t,err)
		state := &notary.TransactorState{
			Signer: signer,
			History: history,
			MutatableTip: chainTip,
			MutatableBlock: block,
			Transaction: test.transaction,
		}

		retState, shouldInterrupt, err := notary.DefaultTransactorRegistry.Distribute(context.Background(), state)
		if test.shouldError {
			assert.NotNil(t, err, test.description)
		} else {
			assert.Nil(t, err, test.description)
		}

		if test.shouldInterrupt {
			assert.True(t, shouldInterrupt, test.description)
		} else {
			assert.False(t, shouldInterrupt, test.description)
		}

		if test.shouldSign {
			mutatedBlock := retState.MutatableBlock
			isSigned,err := signer.IsTransactionSigned(mutatedBlock,test.transaction)
			assert.Nil(t,err, test.description)
			assert.True(t, isSigned, test.description)
		} else {
			mutatedBlock := retState.MutatableBlock
			isSigned,err := signer.IsTransactionSigned(mutatedBlock,test.transaction)
			assert.Nil(t,err, test.description)
			assert.False(t, isSigned, test.description)
		}

	}
}

func TestSendCoinTransactor(t *testing.T) {
	signer := defaultNotary(t)
	//group := signer.Group

	type testDesc struct {
		description string
		transaction *consensuspb.Transaction
		state *notary.TransactorState
		shouldError bool
		shouldInterrupt bool
		shouldSign bool
	}

	type testGenerator func(t *testing.T) *testDesc

	for _,testGen := range []testGenerator{
		func(t *testing.T) *testDesc {
			storage := internalchain.NewMemStorage()
			chain := chainFromEcdsaKey(t, &aliceKey.PublicKey)

			mintTransaction := consensus.EncapsulateTransaction(consensuspb.MINT_COIN, &consensuspb.MintCoinTransaction{
				Name: chain.Id + "catCoin",
				Amount: 1000000000, // 1 BILLION cat cat coin
			})

			sendTransaction := consensus.EncapsulateTransaction(consensuspb.SEND_COIN, &consensuspb.SendCoinTransaction{
				Name: chain.Id + "catCoin",
				Amount: 100,
			})

			block := createBlockWithTransactions(t, []*consensuspb.Transaction{mintTransaction, sendTransaction}, nil)
			chain.Blocks = []*consensuspb.Block{block}
			history := consensus.NewMemoryHistoryStore()

			block,err := signer.SignTransaction(context.Background(), block, mintTransaction)
			assert.Nil(t,err)

			chainTip,err := storage.Get(chain.Id)
			assert.Nil(t,err)
			state := &notary.TransactorState{
				Signer: signer,
				History: history,
				MutatableTip: chainTip,
				MutatableBlock: block,
				Transaction: sendTransaction,
			}

			return &testDesc{
				description: "a genesis where we mint and send in the same block",
				state: state,
				transaction: sendTransaction,
				shouldSign: true,
				shouldInterrupt: false,
				shouldError: false,
			}
		},
		func(t *testing.T) *testDesc {
			storage := internalchain.NewMemStorage()
			chain := chainFromEcdsaKey(t, &aliceKey.PublicKey)

			mintTransaction := consensus.EncapsulateTransaction(consensuspb.MINT_COIN, &consensuspb.MintCoinTransaction{
				Name: chain.Id + "catCoin",
				Amount: 100, // 1 BILLION cat cat coin
			})

			sendTransaction := consensus.EncapsulateTransaction(consensuspb.SEND_COIN, &consensuspb.SendCoinTransaction{
				Name: chain.Id + "catCoin",
				Amount: 101,
			})

			block := createBlockWithTransactions(t, []*consensuspb.Transaction{mintTransaction, sendTransaction}, nil)
			chain.Blocks = []*consensuspb.Block{block}
			history := consensus.NewMemoryHistoryStore()

			block,err := signer.SignTransaction(context.Background(), block, mintTransaction)
			assert.Nil(t,err)

			chainTip,err := storage.Get(chain.Id)
			assert.Nil(t,err)
			state := &notary.TransactorState{
				Signer: signer,
				History: history,
				MutatableTip: chainTip,
				MutatableBlock: block,
				Transaction: sendTransaction,
			}

			return &testDesc{
				description: "with an over spend",
				state: state,
				transaction: sendTransaction,
				shouldSign: false,
				shouldInterrupt: true,
				shouldError: false,
			}
		},
		func(t *testing.T) *testDesc {
			storage := internalchain.NewMemStorage()
			chain := chainFromEcdsaKey(t, &aliceKey.PublicKey)

			mintTransaction := consensus.EncapsulateTransaction(consensuspb.MINT_COIN, &consensuspb.MintCoinTransaction{
				Name: chain.Id + "catCoin",
				Amount: 100, // 1 BILLION cat cat coin
			})

			sendTransaction := consensus.EncapsulateTransaction(consensuspb.SEND_COIN, &consensuspb.SendCoinTransaction{
				Name: chain.Id + "catCoin",
				Amount: 101,
			})

			mintBlock := createBlockWithTransactions(t, []*consensuspb.Transaction{mintTransaction}, nil)
			block := createBlockWithTransactions(t, []*consensuspb.Transaction{sendTransaction}, mintBlock)
			chain.Blocks = []*consensuspb.Block{block}
			history := consensus.NewMemoryHistoryStore()
			history.StoreBlocks([]*consensuspb.Block{mintBlock})

			chainTip,err := storage.Get(chain.Id)
			assert.Nil(t,err)
			state := &notary.TransactorState{
				Signer: signer,
				History: history,
				MutatableTip: chainTip,
				MutatableBlock: block,
				Transaction: sendTransaction,
			}

			return &testDesc{
				description: "when trying to cheat with an unsigned history block",
				state: state,
				transaction: sendTransaction,
				shouldSign: false,
				shouldInterrupt: true,
				shouldError: false,
			}
		},
		func(t *testing.T) *testDesc {
			storage := internalchain.NewMemStorage()
			chain := chainFromEcdsaKey(t, &aliceKey.PublicKey)

			mintTransaction := consensus.EncapsulateTransaction(consensuspb.MINT_COIN, &consensuspb.MintCoinTransaction{
				Name: chain.Id + "catCoin",
				Amount: 100, // 1 BILLION cat cat coin
			})

			sendTransaction := consensus.EncapsulateTransaction(consensuspb.SEND_COIN, &consensuspb.SendCoinTransaction{
				Name: chain.Id + "catCoin",
				Amount: 50,
			})

			sendTransaction2 := consensus.EncapsulateTransaction(consensuspb.SEND_COIN, &consensuspb.SendCoinTransaction{
				Name: chain.Id + "catCoin",
				Amount: 51,
			})

			mintBlock := createBlockWithTransactions(t, []*consensuspb.Transaction{mintTransaction}, nil)
			mintBlock,err := signer.SignBlock(context.Background(), mintBlock)
			assert.Nil(t,err)
			mintBlock,err = signer.Group.ReplaceSignatures(mintBlock)
			assert.Nil(t,err)

			isSigned,_ := signer.Group.IsBlockSigned(mintBlock)
			assert.True(t,isSigned, "signing by group did not sign the mint block")

			block := createBlockWithTransactions(t, []*consensuspb.Transaction{sendTransaction, sendTransaction2}, mintBlock)
			chain.Blocks = []*consensuspb.Block{block}
			history := consensus.NewMemoryHistoryStore()
			history.StoreBlocks([]*consensuspb.Block{mintBlock})

			chainTip,err := storage.Get(chain.Id)
			assert.Nil(t,err)
			state := &notary.TransactorState{
				Signer: signer,
				History: history,
				MutatableTip: chainTip,
				MutatableBlock: block,
				Transaction: sendTransaction2,
			}

			return &testDesc{
				description: "when trying to cheat with too many send_coin in a single new block",
				state: state,
				transaction: sendTransaction,
				shouldSign: false,
				shouldInterrupt: true,
				shouldError: false,
			}
		},
		func(t *testing.T) *testDesc {
			storage := internalchain.NewMemStorage()
			chain := chainFromEcdsaKey(t, &aliceKey.PublicKey)

			mintTransaction := consensus.EncapsulateTransaction(consensuspb.MINT_COIN, &consensuspb.MintCoinTransaction{
				Name: chain.Id + "catCoin",
				Amount: 101, // 1 BILLION cat cat coin
			})

			sendTransaction := consensus.EncapsulateTransaction(consensuspb.SEND_COIN, &consensuspb.SendCoinTransaction{
				Name: chain.Id + "catCoin",
				Amount: 100,
			})

			mintBlock := createBlockWithTransactions(t, []*consensuspb.Transaction{mintTransaction}, nil)
			mintBlock,err := signer.SignBlock(context.Background(), mintBlock)
			assert.Nil(t,err)
			mintBlock,err = signer.Group.ReplaceSignatures(mintBlock)
			assert.Nil(t,err)

			isSigned,_ := signer.Group.IsBlockSigned(mintBlock)
			assert.True(t,isSigned, "signing by group did not sign the mint block")

			block := createBlockWithTransactions(t, []*consensuspb.Transaction{sendTransaction}, mintBlock)
			chain.Blocks = []*consensuspb.Block{block}

			history := consensus.NewMemoryHistoryStore()
			history.StoreBlocks([]*consensuspb.Block{mintBlock})

			chainTip,err := storage.Get(chain.Id)
			assert.Nil(t,err)
			state := &notary.TransactorState{
				Signer: signer,
				History: history,
				MutatableTip: chainTip,
				MutatableBlock: block,
				Transaction: sendTransaction,
			}

			return &testDesc{
				description: "when the mint transaction is in an older block",
				state: state,
				transaction: sendTransaction,
				shouldSign: true,
				shouldInterrupt: false,
				shouldError: false,
			}
		},
		func(t *testing.T) *testDesc {
			storage := internalchain.NewMemStorage()
			chain := chainFromEcdsaKey(t, &aliceKey.PublicKey)

			mintTransaction := consensus.EncapsulateTransaction(consensuspb.MINT_COIN, &consensuspb.MintCoinTransaction{
				Name: chain.Id + "catCoin",
				Amount: 100, // 1 BILLION cat cat coin
			})

			sendTransaction := consensus.EncapsulateTransaction(consensuspb.SEND_COIN, &consensuspb.SendCoinTransaction{
				Name: chain.Id + "catCoin",
				Amount: 101,
			})

			mintBlock := createBlockWithTransactions(t, []*consensuspb.Transaction{mintTransaction}, nil)
			block := createBlockWithTransactions(t, []*consensuspb.Transaction{sendTransaction}, mintBlock)

			chain.Blocks = []*consensuspb.Block{mintBlock, block}
			history := consensus.NewMemoryHistoryStore()

			block,err := signer.SignTransaction(context.Background(), block, mintTransaction)
			assert.Nil(t,err)

			chainTip,err := storage.Get(chain.Id)
			assert.Nil(t,err)
			state := &notary.TransactorState{
				Signer: signer,
				History: history,
				MutatableTip: chainTip,
				MutatableBlock: block,
				Transaction: sendTransaction,
			}

			return &testDesc{
				description: "with an over spend",
				state: state,
				transaction: sendTransaction,
				shouldSign: false,
				shouldInterrupt: true,
				shouldError: false,
			}
		},
	} {
		log.Root().SetHandler(log.LvlFilterHandler(log.Lvl(log.LvlDebug), log.StreamHandler(os.Stderr, log.TerminalFormat(false))))

		test := testGen(t)
		t.Logf("chainId: %v", test.state.MutatableTip.Id)
		retState, shouldInterrupt, err := notary.DefaultTransactorRegistry.Distribute(context.Background(), test.state)
		if test.shouldError {
			assert.NotNil(t, err, test.description)
		} else {
			assert.Nil(t, err, test.description)
		}

		if test.shouldInterrupt {
			assert.True(t, shouldInterrupt, test.description)
		} else {
			assert.False(t, shouldInterrupt, test.description)
		}

		if test.shouldSign {
			mutatedBlock := retState.MutatableBlock
			isSigned,err := signer.IsTransactionSigned(mutatedBlock,test.transaction)
			assert.Nil(t,err, test.description)
			assert.True(t, isSigned, test.description)
		} else {
			mutatedBlock := retState.MutatableBlock
			isSigned,err := signer.IsTransactionSigned(mutatedBlock,test.transaction)
			assert.Nil(t,err, test.description)
			assert.False(t, isSigned, test.description)
		}
	}
}