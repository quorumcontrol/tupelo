package nodebuilder

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/quorumcontrol/tupelo/sdk/gossip/types"
	"github.com/quorumcontrol/tupelo/server/testnotarygroup"
	"github.com/stretchr/testify/require"
)

func TestLocalNetwork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	nodeCount := 3

	ts := testnotarygroup.NewTestSet(t, nodeCount)

	privateKeySets := make([]*PrivateKeySet, len(ts.EcdsaKeys))
	for i, signKey := range ts.SignKeys {
		privSet := &PrivateKeySet{
			SignKey: signKey,
			DestKey: ts.EcdsaKeys[i],
		}
		privateKeySets[i] = privSet
	}

	ln, err := NewLocalNetwork(ctx, "testnamespace", privateKeySets, types.DefaultConfig())
	require.Nil(t, err)

	assert.Len(t, ln.Builders, nodeCount)
	assert.Len(t, ln.Nodes, nodeCount)
}
