package types

import (
	"testing"

	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestActorAddress(t *testing.T) {
	ts := testnotarygroup.NewTestSet(t, 1)
	signer1 := NewLocalSigner(ts.PubKeys[0].ToEcdsaPub(), ts.SignKeys[0])
	require.NotEmpty(t, signer1.ActorAddress())
	assert.Equal(t, hexutil.Encode(crypto.FromECDSAPub(ts.PubKeys[0].ToEcdsaPub())), signer1.ActorAddress())
}
