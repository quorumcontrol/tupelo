package actors

import (
	"context"
	"testing"

	"github.com/quorumcontrol/messages/v2/build/go/signatures"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/middleware"
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
		defer actor.EmptyRootContext.Stop(alwaysVerifier)

		validator := newCommitValidator(notaryGroup, alwaysVerifier)
		currentState := &signatures.TreeState{
			Signature: &signatures.Signature{
				Signers: []uint32{1, 1, 0},
			},
		}

		require.True(t, validator.validate(ctx, "", currentState))
	})

	t.Run("fails with invalid signatures", func(t *testing.T) {
		// stub out actual signature verification
		neverVerifier := actor.EmptyRootContext.SpawnPrefix(NewNeverVerifierProps(), "alwaysVerifier")
		defer actor.EmptyRootContext.Stop(neverVerifier)

		validator := newCommitValidator(notaryGroup, neverVerifier)

		currentState := &signatures.TreeState{
			Signature: &signatures.Signature{
				Signers: []uint32{1, 1, 0},
			},
		}

		require.False(t, validator.validate(ctx, "", currentState))
	})

	t.Run("fails with not enough signatures", func(t *testing.T) {
		// stub out actual signature verification
		alwaysVerifier := actor.EmptyRootContext.SpawnPrefix(NewAlwaysVerifierProps(), "alwaysVerifier")
		defer actor.EmptyRootContext.Stop(alwaysVerifier)

		validator := newCommitValidator(notaryGroup, alwaysVerifier)

		currentState := &signatures.TreeState{
			Signature: &signatures.Signature{
				Signers: []uint32{1, 0, 0},
			},
		}

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
