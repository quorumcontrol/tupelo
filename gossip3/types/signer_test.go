package types

import (
	"testing"

	"github.com/quorumcontrol/tupelo/p2p"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActorAddress(t *testing.T) {
	ts := testnotarygroup.NewTestSet(t, 1)
	signer1 := NewLocalSigner(ts.PubKeys[0].ToEcdsaPub(), ts.SignKeys[0])
	require.NotEmpty(t, signer1.ActorAddress())
	peer, err := p2p.PeerFromEcdsaKey(ts.PubKeys[0].ToEcdsaPub())
	require.Nil(t, err)
	assert.Equal(t, peer.Pretty(), signer1.ActorAddress())
}
