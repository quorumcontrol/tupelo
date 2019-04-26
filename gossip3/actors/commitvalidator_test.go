package actors

import (
	"context"
	"testing"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/Workiva/go-datastructures/bitarray"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/require"
)

func TestCommitValidator(t *testing.T) {
	numMembers := 3
	ts := testnotarygroup.NewTestSet(t, numMembers)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	notaryGroup, _, err := newTupeloSystem(ctx, ts)
	require.Nil(t, err)

	t.Run("passes with everything valid", func(t *testing.T) {
		// stub out actual signature verification
		alwaysVerifier := actor.EmptyRootContext.SpawnPrefix(NewAlwaysVerifierProps(), "alwaysVerifier")
		defer alwaysVerifier.Stop()

		validator := newCommitValidator(notaryGroup, alwaysVerifier)

		arry := bitarray.NewSparseBitArray()
		err := arry.SetBit(0)
		require.Nil(t, err)
		err = arry.SetBit(1)
		require.Nil(t, err)

		marshaledArray, err := bitarray.Marshal(arry)
		require.Nil(t, err)
		currentState := &extmsgs.CurrentState{
			Signature: &extmsgs.Signature{
				Signers: marshaledArray,
			},
		}

		require.True(t, validator.validate(ctx, "", currentState))
	})

	t.Run("fails with invalid signatures", func(t *testing.T) {
		// stub out actual signature verification
		neverVerifier := actor.EmptyRootContext.SpawnPrefix(NewNeverVerifierProps(), "alwaysVerifier")
		defer neverVerifier.Stop()

		validator := newCommitValidator(notaryGroup, neverVerifier)

		arry := bitarray.NewSparseBitArray()
		err := arry.SetBit(0)
		require.Nil(t, err)
		err = arry.SetBit(1)
		require.Nil(t, err)

		marshaledArray, err := bitarray.Marshal(arry)
		require.Nil(t, err)
		currentState := &extmsgs.CurrentState{
			Signature: &extmsgs.Signature{
				Signers: marshaledArray,
			},
		}

		require.False(t, validator.validate(ctx, "", currentState))
	})

	t.Run("fails with not enough signatures", func(t *testing.T) {
		// stub out actual signature verification
		alwaysVerifier := actor.EmptyRootContext.SpawnPrefix(NewAlwaysVerifierProps(), "alwaysVerifier")
		defer alwaysVerifier.Stop()

		validator := newCommitValidator(notaryGroup, alwaysVerifier)

		arry := bitarray.NewSparseBitArray()
		err := arry.SetBit(0)
		require.Nil(t, err)

		marshaledArray, err := bitarray.Marshal(arry)
		require.Nil(t, err)
		currentState := &extmsgs.CurrentState{
			Signature: &extmsgs.Signature{
				Signers: marshaledArray,
			},
		}

		require.False(t, validator.validate(ctx, "", currentState))
	})

	t.Run("fails the third time through", func(t *testing.T) {
		// stub out actual signature verification
		alwaysVerifier := actor.EmptyRootContext.SpawnPrefix(NewAlwaysVerifierProps(), "alwaysVerifier")
		defer alwaysVerifier.Stop()

		validator := newCommitValidator(notaryGroup, alwaysVerifier)

		arry := bitarray.NewSparseBitArray()

		err := arry.SetBit(0)
		require.Nil(t, err)
		err = arry.SetBit(1)
		require.Nil(t, err)

		marshaledArray, err := bitarray.Marshal(arry)
		require.Nil(t, err)
		currentState := &extmsgs.CurrentState{
			Signature: &extmsgs.Signature{
				ObjectID: []byte("object"),
				NewTip:   []byte("newtip"),
				Signers:  marshaledArray,
			},
		}

		// first time through it's all good
		require.True(t, validator.validate(ctx, "", currentState))

		// second time through it's all good
		require.True(t, validator.validate(ctx, "", currentState))

		// third time fails because we've already seen the commit
		require.False(t, validator.validate(ctx, "", currentState))
	})

}

type NeverVerifier struct {
	middleware.LogAwareHolder
}

func NewNeverVerifierProps() *actor.Props {
	return actor.PropsFromProducer(func() actor.Actor {
		return new(NeverVerifier)
	}).WithReceiverMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (nv *NeverVerifier) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.SignatureVerification:
		msg.Verified = false
		context.Respond(msg)
	}
}
