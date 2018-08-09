package consensus

import (
	"testing"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/stretchr/testify/require"
)

func TestSignedChainTree_IsGenesis(t *testing.T) {
	key, err := crypto.GenerateKey()
	require.Nil(t, err)

	newTree, err := NewSignedChainTree(key.PublicKey)
	require.Nil(t, err)

	require.True(t, newTree.IsGenesis())

	unsignedBlock := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				{
					Type: TransactionTypeSetData,
					Payload: &SetDataPayload{
						Path:  "test",
						Value: "value",
					},
				},
			},
		},
	}

	blockWithHeaders, err := SignBlock(unsignedBlock, key)
	require.Nil(t, err)

	isValid, err := newTree.ChainTree.ProcessBlock(blockWithHeaders)
	require.True(t, isValid)

	require.False(t, newTree.IsGenesis())

}
