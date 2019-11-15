package actors

import (
	"strconv"
	"testing"
	"time"

	"github.com/quorumcontrol/messages/v2/build/go/signatures"
	sigfuncs "github.com/quorumcontrol/tupelo-go-sdk/signatures"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/quorumcontrol/tupelo-go-sdk/bls"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerification(t *testing.T) {
	ts := testnotarygroup.NewTestSet(t, 1)
	rootContext := actor.EmptyRootContext
	ss := rootContext.Spawn(NewSignatureVerifier())
	defer rootContext.Poison(ss)

	msg := crypto.Keccak256([]byte("hi"))
	sig, err := sigfuncs.BLSSign(ts.SignKeys[0], msg, 1, 0)
	require.Nil(t, err)

	resp, err := rootContext.RequestFuture(ss, &messages.SignatureVerification{
		Message:   msg,
		Signature: sig,
		VerKeys:   []*bls.VerKey{ts.VerKeys[0]},
	}, 1*time.Second).Result()

	require.Nil(t, err)
	assert.True(t, resp.(*messages.SignatureVerification).Verified)
}

type benchTestSigHolder struct {
	msg []byte
	sig *signatures.Signature
}

func BenchmarkVerification(b *testing.B) {
	ts := testnotarygroup.NewTestSet(b, 1)
	rootContext := actor.EmptyRootContext
	ss := rootContext.Spawn(NewSignatureVerifier())
	defer rootContext.Stop(ss)

	futures := make([]*actor.Future, b.N)

	sigs := make([]*benchTestSigHolder, b.N)
	for i := 0; i < b.N; i++ {
		msg := crypto.Keccak256([]byte("hi" + strconv.Itoa(i)))
		sig, err := sigfuncs.BLSSign(ts.SignKeys[0], msg, 1, 0)
		require.Nil(b, err)
		sigs[i] = &benchTestSigHolder{
			msg: msg,
			sig: sig,
		}
	}
	b.ResetTimer()

	for i, sigHolder := range sigs {
		f := rootContext.RequestFuture(ss, &messages.SignatureVerification{
			Message:   sigHolder.msg,
			Signature: sigHolder.sig,
			VerKeys:   []*bls.VerKey{ts.VerKeys[0]},
		}, 5*time.Second)
		futures[i] = f
	}

	for _, f := range futures {
		resp, err := f.Result()
		require.Nil(b, err)
		assert.True(b, resp.(*messages.SignatureVerification).Verified)
	}
}
