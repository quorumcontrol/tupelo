package types

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNotaryGroupBlockValidators(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// notary group with default config:
	ng := NewNotaryGroup("TestNotaryGroupBlockValidators")

	validators, err := ng.BlockValidators(ctx)
	require.Nil(t, err)
	assert.NotEmpty(t, validators)
	assert.Len(t, validators, len(ng.config.ValidatorGenerators))
}
