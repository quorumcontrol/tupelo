package consensus

import (
	"testing"
	"time"

	"github.com/quorumcontrol/qc3/bls"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/chaintree/nodestore"
	"github.com/quorumcontrol/storage"
)

func TestNotaryGroupCreateBlockFor(t *testing.T) {
	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())

	key, err := crypto.GenerateKey()
	require.Nil(t, err)
	id := AddrToDid(crypto.PubkeyToAddress(key.PublicKey).String())
	group := NewNotaryGroup(id, nodeStore)

	rnDstKey, err := crypto.GenerateKey()
	rnKey := bls.MustNewSignKey()

	rn := NewRemoteNode(BlsKeyToPublicKey(rnKey.MustVerKey()), EcdsaToPublicKey(&rnDstKey.PublicKey))

	block, err := group.CreateBlockFor(1, []*RemoteNode{rn})
	require.Nil(t, err)
	assert.NotNil(t, block)
}

func TestNotaryGroupAddBlock(t *testing.T) {
	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())

	key, err := crypto.GenerateKey()
	require.Nil(t, err)
	id := AddrToDid(crypto.PubkeyToAddress(key.PublicKey).String())
	group := NewNotaryGroup(id, nodeStore)

	rnDstKey, err := crypto.GenerateKey()
	rnKey := bls.MustNewSignKey()

	rn := NewRemoteNode(BlsKeyToPublicKey(rnKey.MustVerKey()), EcdsaToPublicKey(&rnDstKey.PublicKey))

	block, err := group.CreateBlockFor(1, []*RemoteNode{rn})
	require.Nil(t, err)
	assert.NotNil(t, block)

	err = group.AddBlock(block)
	assert.Nil(t, err)
}

func TestNotaryGroupRoundInfoFor(t *testing.T) {
	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())

	key, err := crypto.GenerateKey()
	require.Nil(t, err)
	id := AddrToDid(crypto.PubkeyToAddress(key.PublicKey).String())
	group := NewNotaryGroup(id, nodeStore)

	rnDstKey, err := crypto.GenerateKey()
	rnKey := bls.MustNewSignKey()

	rn := NewRemoteNode(BlsKeyToPublicKey(rnKey.MustVerKey()), EcdsaToPublicKey(&rnDstKey.PublicKey))

	block, err := group.CreateBlockFor(1, []*RemoteNode{rn})
	require.Nil(t, err)
	require.NotNil(t, block)

	err = group.AddBlock(block)
	require.Nil(t, err)

	roundInfo, err := group.RoundInfoFor(1)
	require.Nil(t, err)
	require.NotNil(t, roundInfo)
	assert.Equal(t, []*RemoteNode{rn}, roundInfo.Signers)
}
func TestNotaryGroupMostRecentRoundInfo(t *testing.T) {
	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())

	key, err := crypto.GenerateKey()
	require.Nil(t, err)
	id := AddrToDid(crypto.PubkeyToAddress(key.PublicKey).String())
	group := NewNotaryGroup(id, nodeStore)

	rnDstKey, err := crypto.GenerateKey()
	rnKey := bls.MustNewSignKey()

	rn := NewRemoteNode(BlsKeyToPublicKey(rnKey.MustVerKey()), EcdsaToPublicKey(&rnDstKey.PublicKey))

	block, err := group.CreateBlockFor(1, []*RemoteNode{rn})
	require.Nil(t, err)
	require.NotNil(t, block)

	err = group.AddBlock(block)
	require.Nil(t, err)

	roundInfo, err := group.MostRecentRoundInfo(5)
	require.Nil(t, err)
	assert.Equal(t, []*RemoteNode{rn}, roundInfo.Signers)
}

func TestNotaryGroupCreateGenesisState(t *testing.T) {
	nodeStore := nodestore.NewStorageBasedStore(storage.NewMemStorage())

	key, err := crypto.GenerateKey()
	require.Nil(t, err)
	id := AddrToDid(crypto.PubkeyToAddress(key.PublicKey).String())
	group := NewNotaryGroup(id, nodeStore)

	rnDstKey, err := crypto.GenerateKey()
	rnKey := bls.MustNewSignKey()

	round := group.RoundAt(time.Now())

	rn := NewRemoteNode(BlsKeyToPublicKey(rnKey.MustVerKey()), EcdsaToPublicKey(&rnDstKey.PublicKey))
	group.CreateGenesisState(round, rn)

	roundInfo, err := group.RoundInfoFor(round)
	require.Nil(t, err)
	assert.Equal(t, []*RemoteNode{rn}, roundInfo.Signers)

	// check 2nd rounds mostrecent works
	roundInfo, err = group.MostRecentRoundInfo(round + 1)
	require.Nil(t, err)
	assert.Equal(t, []*RemoteNode{rn}, roundInfo.Signers)
}
