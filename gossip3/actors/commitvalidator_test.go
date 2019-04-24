package actors

import (
	"context"
	"testing"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/Workiva/go-datastructures/bitarray"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo/testnotarygroup"
	"github.com/stretchr/testify/require"
)

func TestCommitValidator(t *testing.T) {
	numMembers := 3
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ts := testnotarygroup.NewTestSet(t, numMembers)

	t.Run("with everything valid", func(t *testing.T) {
		notaryGroup, _, err := newTupeloSystem(ctx, ts)
		require.Nil(t, err)

		// stub out actual signature verification
		alwaysVerifier := actor.EmptyRootContext.SpawnPrefix(NewAlwaysVerifierProps(), "alwaysVerifier")
		defer alwaysVerifier.Stop()

		validator := newCommitValidator(notaryGroup, alwaysVerifier)

		arry := bitarray.NewSparseBitArray()
		arry.SetBit(0)
		arry.SetBit(1)
		marshaledArray, err := bitarray.Marshal(arry)
		require.Nil(t, err)
		currentState := &extmsgs.CurrentState{
			Signature: &extmsgs.Signature{
				Signers: marshaledArray,
			},
		}

		require.True(t, validator.validate(ctx, "", currentState))
	})

}
