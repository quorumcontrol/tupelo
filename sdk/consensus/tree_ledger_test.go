package consensus

import (
	"context"
	"strings"
	"testing"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/quorumcontrol/messages/v2/build/go/transactions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/quorumcontrol/chaintree/chaintree"
	"github.com/quorumcontrol/chaintree/nodestore"
)

func NewTestChainTree(t *testing.T, ctx context.Context) *chaintree.ChainTree {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	key, err := crypto.GenerateKey()
	assert.Nil(t, err)

	store := nodestore.MustMemoryStore(context.TODO())
	treeDID := AddrToDid(crypto.PubkeyToAddress(key.PublicKey).String())
	emptyTree := NewEmptyTree(ctx, treeDID, store)

	testTree, err := chaintree.NewChainTree(ctx, emptyTree, nil, DefaultTransactors)
	assert.Nil(t, err)

	return testTree
}

func TestTreeLedger(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	testTree := NewTestChainTree(t, ctx)
	treeDID, err := testTree.Id(ctx)
	assert.Nil(t, err)

	tokenLocalName := "test-token"
	tokenFullName := strings.Join([]string{treeDID, tokenLocalName}, ":")
	tokenName := &TokenName{ChainTreeDID: treeDID, LocalName: tokenLocalName}

	// Test NewTreeLedger
	testLedger := NewTreeLedger(testTree.Dag, tokenName)
	assert.Equal(t, testLedger.tokenName.LocalName, tokenLocalName)
	assert.Equal(t, testLedger.tokenName.String(), tokenFullName)

	// Test EstablishToken
	tokenExists, err := testLedger.TokenExists()
	require.Nil(t, err)
	assert.False(t, tokenExists)

	maximum := uint64(42)
	monetaryPolicy := transactions.TokenMonetaryPolicy{Maximum: maximum}
	newTree, err := testLedger.EstablishToken(monetaryPolicy)
	require.Nil(t, err)

	newTestLedger := NewTreeLedger(newTree, tokenName)

	newTokenExists, err := newTestLedger.TokenExists()
	require.Nil(t, err)
	assert.True(t, newTokenExists)

	// Test MintToken
	// cannot mint more than monetary policy allows
	_, err = newTestLedger.MintToken(maximum + 1)
	assert.NotNil(t, err)

	mintAmount := uint64(25)
	newTree, err = newTestLedger.MintToken(mintAmount)
	require.Nil(t, err)

	newTestLedger = NewTreeLedger(newTree, tokenName)
	balance, err := newTestLedger.Balance()
	require.Nil(t, err)
	assert.Equal(t, mintAmount, balance)

	// Test SendToken
	sendAmount := uint64(5)
	newTree, err = newTestLedger.SendToken("test-tx-id", "did:tupelo:othertestchaintree", sendAmount)
	require.Nil(t, err)

	newTestLedger = NewTreeLedger(newTree, tokenName)

	balance, err = newTestLedger.Balance()
	require.Nil(t, err)
	assert.Equal(t, mintAmount-sendAmount, balance)

	// Test ReceiveToken
	recipientTree := NewTestChainTree(t, ctx)

	recipientLedger := NewTreeLedger(recipientTree.Dag, tokenName)
	recipientTokenExists, err := recipientLedger.TokenExists()
	require.Nil(t, err)
	assert.False(t, recipientTokenExists)

	receiveAmount := uint64(5)
	newRecipientTree, err := recipientLedger.ReceiveToken("test-send-token", receiveAmount)
	require.Nil(t, err)

	newRecipientLedger := NewTreeLedger(newRecipientTree, tokenName)

	newRecipientTokenExists, err := newRecipientLedger.TokenExists()
	require.Nil(t, err)
	assert.True(t, newRecipientTokenExists)

	newRecipientBalance, err := newRecipientLedger.Balance()
	require.Nil(t, err)
	assert.Equal(t, newRecipientBalance, receiveAmount)
}
