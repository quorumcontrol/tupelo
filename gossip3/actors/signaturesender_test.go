package actors

import (
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	extmsgs "github.com/quorumcontrol/tupelo-go-sdk/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendSigs(t *testing.T) {
	ts := testnotarygroup.NewTestSet(t, 1)
	rootContext := actor.EmptyRootContext
	ss := rootContext.Spawn(NewSignatureSenderProps())
	defer ss.Poison()

	fut := actor.NewFuture(5 * time.Second)
	subscriberFunc := func(context actor.Context) {
		switch msg := context.Message().(type) {
		case *extmsgs.Signature:
			context.Send(fut.PID(), msg)
		}
	}

	subscriber := rootContext.Spawn(actor.PropsFromFunc(subscriberFunc))
	defer subscriber.Poison()

	signer := types.NewLocalSigner(ts.PubKeys[0].ToEcdsaPub(), ts.SignKeys[0])
	signer.Actor = subscriber

	rootContext.Send(ss, &messages.SignatureWrapper{
		Signature:        &extmsgs.Signature{TransactionID: []byte("testonly")},
		RewardsCommittee: []*types.Signer{signer},
	})

	msg, err := fut.Result()
	require.Nil(t, err)
	assert.Equal(t, []byte("testonly"), msg.(*extmsgs.Signature).TransactionID)
}
