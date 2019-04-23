package actors

import (
	"context"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/Workiva/go-datastructures/bitarray"
	peer "github.com/libp2p/go-libp2p-peer"
	extmsgs "github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/types"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
)

// I think if we had a validator that a) made sure the only publisher was the notary group, b) the commit is valid (sigs and stuff) and c) wasn't a repeat commit that had already been seen that we could move the incoming commit validation work to the gossipsub
// and then in the app layer we could just treat commit messages as "good"

type commitValidator struct {
	notaryGroup      *types.NotaryGroup
	signatureChecker *actor.PID
	log              *zap.SuggaredLogger
}

func (cv *commitValidator) validate(ctx context.Context, p peer.ID, msg extmsgs.WireMessage) bool {
	currState := msg.(*extmsgs.CurrentState)

	currWrapper := &messages.CurrentStateWrapper{
		CurrentState: currState,
		Internal:     false,
		// Key:          msg.store.Key,
		// Value:        msg.store.Value,
		Metadata: messages.MetadataMap{"seen": time.Now()},
	}

	sig := currWrapper.CurrentState.Signature
	signerArray, err := bitarray.Unmarshal(sig.Signers)
	if err != nil {
		// return fmt.Errorf("error unmarshaling: %v", err)
		return false
	}
	var verKeys [][]byte

	signers := cv.notaryGroup.AllSigners()
	for i, signer := range signers {
		isSet, err := signerArray.GetBit(uint64(i))
		if err != nil {
			// return fmt.Errorf("error getting bit: %v", err)
			return false
		}
		if isSet {
			verKeys = append(verKeys, signer.VerKey.Bytes())
		}
	}

	cv.log.Debugw("checking signature", "numVerKeys", len(verKeys))
	context.RequestWithCustomSender(csw.signatureChecker, &messages.SignatureVerification{
		Message:   sig.GetSignable(),
		Signature: sig.Signature,
		VerKeys:   verKeys,
		Memo:      currWrapper,
	}, csw.router)

}
