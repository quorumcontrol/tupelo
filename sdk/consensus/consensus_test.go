package consensus

import (
	"context"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/messages/v2/build/go/transactions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsBlockSignedBy(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	key, err := crypto.GenerateKey()
	assert.Nil(t, err)

	txn, err := chaintree.NewSetDataTransaction("down/in/the/thing", "hi")
	assert.Nil(t, err)

	blockWithHeaders := chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip:  nil,
			Height:       0,
			Transactions: []*transactions.Transaction{txn},
		},
	}

	signed, err := SignBlock(ctx, &blockWithHeaders, key)

	assert.Nil(t, err)

	isSigned, err := IsBlockSignedBy(ctx, signed, crypto.PubkeyToAddress(key.PublicKey).String())

	assert.Nil(t, err)
	assert.True(t, isSigned)

}

func TestPassPhraseKey(t *testing.T) {
	key, err := PassPhraseKey([]byte("secretPassword"), []byte("salt"))
	require.Nil(t, err)
	assert.Equal(t, []byte{0x5b, 0x94, 0x85, 0x8e, 0xda, 0x63, 0xd4, 0xe8, 0x12, 0xa5, 0xed, 0x98, 0xa2, 0xe0, 0xc8, 0xe0, 0xef, 0xcb, 0xf2, 0x72, 0x69, 0xca, 0xa2, 0x9d, 0xe9, 0x6c, 0x7a, 0x93, 0xcc, 0x73, 0x9, 0x14}, crypto.FromECDSA(key))
}
