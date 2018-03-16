package notary_test

import (
	"testing"
	"github.com/quorumcontrol/qc3/notary"
	"github.com/quorumcontrol/qc3/consensus"
	"github.com/quorumcontrol/qc3/consensus/consensuspb"
	"context"
	"github.com/stretchr/testify/assert"
	"github.com/quorumcontrol/qc3/internalchain"
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