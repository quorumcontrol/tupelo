package consensus

import (
	"testing"
	"github.com/stretchr/testify/assert"
	"github.com/quorumcontrol/qc3/bls"
	"github.com/quorumcontrol/qc3/internalchain"
)

func TestNewNotary(t *testing.T) {
	key,err := bls.NewSignKey()
	assert.Nil(t, err)

	storage := internalchain.NewMemStorage()

	notary := NewNotary(storage, key)
	assert.Equal(t, notary.ChainStore, storage)
}
