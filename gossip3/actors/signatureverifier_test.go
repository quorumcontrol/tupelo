package actors

import (
	"strconv"
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
	rootContext := actor.EmptyRootContext
	ss := rootContext.Spawn(NewSignatureVerifier())
	defer ss.Poison()

	msg := crypto.Keccak256([]byte("hi"))
	sig, err := ts.SignKeys[0].Sign(msg)
	require.Nil(t, err)

	resp, err := rootContext.RequestFuture(ss, &messages.SignatureVerification{
		Message:   msg,
		Signature: sig,
		VerKeys:   [][]byte{ts.VerKeys[0].Bytes()},
	}, 1*time.Second).Result()

	require.Nil(t, err)
	assert.True(t, resp.(*messages.SignatureVerification).Verified)
}

type benchTestSigHolder struct {
	msg []byte
	sig []byte
}

func BenchmarkVerification(b *testing.B) {
	ts := testnotarygroup.NewTestSet(b, 1)
	rootContext := actor.EmptyRootContext
	ss := rootContext.Spawn(NewSignatureVerifier())
	defer ss.Stop()

	futures := make([]*actor.Future, b.N)

	sigs := make([]*benchTestSigHolder, b.N)
	for i := 0; i < b.N; i++ {
		msg := crypto.Keccak256([]byte("hi" + strconv.Itoa(i)))
		sig, err := ts.SignKeys[0].Sign(msg)
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
			VerKeys:   [][]byte{ts.VerKeys[0].Bytes()},
		}, 5*time.Second)
		futures[i] = f
	}

	for _, f := range futures {
		resp, err := f.Result()
		require.Nil(b, err)
		assert.True(b, resp.(*messages.SignatureVerification).Verified)
	}
}
