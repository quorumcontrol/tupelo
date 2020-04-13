package types

import (
	"io/ioutil"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfig(t *testing.T) {
	c := DefaultConfig()
	assert.NotEmpty(t, c.ValidatorGenerators)
	assert.NotEmpty(t, c.Transactions)
}

func TestGenerateNotaryGroup(t *testing.T) {
	bits, err := ioutil.ReadFile("example.toml")
	require.Nil(t, err)
	c, err := TomlToConfig(string(bits))
	assert.Nil(t, err)

	t.Run("without a local signer", func(t *testing.T) {
		ng, err := c.NotaryGroup(nil)
		require.Nil(t, err)
		assert.Len(t, ng.Signers, 2)

		for i, s := range ng.AllSigners() {
			assert.Nilf(t, s.Actor, "actor at index %d", i)
		}
	})

	t.Run("with a local signer", func(t *testing.T) {
		setupNg, err := c.NotaryGroup(nil)
		require.Nil(t, err)

		ng, err := c.NotaryGroup(setupNg.SignerAtIndex(0))
		require.Nil(t, err)

		assert.Len(t, ng.Signers, 2)
		assert.Nil(t, ng.SignerAtIndex(0).Actor)
		assert.NotNil(t, ng.SignerAtIndex(1).Actor)
	})
}
