package consensus

import (
	"testing"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/assert"
	"github.com/quorumcontrol/chaintree/chaintree"
)

func TestIsBlockSignedBy(t *testing.T) {
	key,err := crypto.GenerateKey()
	assert.Nil(t,err)

	blockWithHeaders := &chaintree.BlockWithHeaders{
		Block: chaintree.Block{
			PreviousTip: "",
			Transactions: []*chaintree.Transaction{
				{
					Type: "SET_DATA",
					Payload: map[string]string{
						"path": "down/in/the/thing",
						"value": "hi",
					},
				},
			},
		},
	}

	signed,err := SignBlock(blockWithHeaders, key)

	assert.Nil(t, err)

	isSigned,err := IsBlockSignedBy(signed, crypto.PubkeyToAddress(key.PublicKey).String())

	assert.Nil(t,err)
	assert.True(t, isSigned)

}
