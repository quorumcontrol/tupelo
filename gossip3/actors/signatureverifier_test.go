package actors

import (
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerification(t *testing.T) {
	ts := testnotarygroup.NewTestSet(t, 1)
	ss := actor.Spawn(NewSignatureVerifier())
	defer ss.Poison()

	msg := crypto.Keccak256([]byte("hi"))
	sig, err := ts.SignKeys[0].Sign(msg)
	require.Nil(t, err)

	resp, err := ss.RequestFuture(&messages.SignatureVerification{
		Message:   msg,
		Signature: sig,
		VerKeys:   [][]byte{ts.VerKeys[0].Bytes()},
	}, 1*time.Second).Result()

	require.Nil(t, err)
	assert.True(t, resp.(*messages.SignatureVerification).Verified)
}
