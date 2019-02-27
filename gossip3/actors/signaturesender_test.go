package actors

import (
	"testing"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/types"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendSigs(t *testing.T) {
	ts := testnotarygroup.NewTestSet(t, 1)
	ss := actor.Spawn(NewSignatureSenderProps())
	defer ss.Poison()

	fut := actor.NewFuture(5 * time.Second)
	subscriberFunc := func(context actor.Context) {
		switch msg := context.Message().(type) {
		case *messages.Signature:
			fut.PID().Tell(msg)
		}
	}

	subscriber := actor.Spawn(actor.FromFunc(subscriberFunc))
	defer subscriber.Poison()

	signer := types.NewLocalSigner(ts.PubKeys[0].ToEcdsaPub(), ts.SignKeys[0])
	signer.Actor = subscriber

	ss.Tell(&messages.SignatureWrapper{
		Signature:        &messages.Signature{TransactionID: []byte("testonly")},
		RewardsCommittee: []*types.Signer{signer},
	})

	msg, err := fut.Result()
	require.Nil(t, err)
	assert.Equal(t, []byte("testonly"), msg.(*messages.Signature).TransactionID)
}
