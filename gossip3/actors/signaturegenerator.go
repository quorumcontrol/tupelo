package actors

import (
	"fmt"
	"time"

	sigfuncs "github.com/quorumcontrol/tupelo-go-sdk/signatures"

	"github.com/quorumcontrol/messages/v2/build/go/signatures"
	"github.com/quorumcontrol/tupelo-go-sdk/consensus"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/AsynkronIT/protoactor-go/router"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-sdk/gossip3/types"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
)

type SignatureGenerator struct {
	middleware.LogAwareHolder

	notaryGroup *types.NotaryGroup
	signer      *types.Signer
}

const generatorConcurrency = 200

func NewSignatureGeneratorProps(self *types.Signer, ng *types.NotaryGroup) *actor.Props {
	return router.NewRoundRobinPool(generatorConcurrency).WithProducer(func() actor.Actor {
		return &SignatureGenerator{
			signer:      self,
			notaryGroup: ng,
		}
	}).WithReceiverMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (sg *SignatureGenerator) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.TransactionWrapper:
		sg.handleNewTransaction(context, msg)
	}
}

func (sg *SignatureGenerator) handleNewTransaction(context actor.Context, msg *messages.TransactionWrapper) {
	ng := sg.notaryGroup

	committee, err := ng.RewardsCommittee([]byte(msg.Transaction.NewTip), sg.signer)
	if err != nil {
		panic(fmt.Sprintf("error getting committee: %v", err))
	}

	state := &signatures.TreeState{
		TransactionId: msg.TransactionId,
		ObjectId:      msg.Transaction.ObjectId,
		PreviousTip:   msg.Transaction.PreviousTip,
		NewTip:        msg.Transaction.NewTip,
		Height:        msg.Transaction.Height,
	}

	sg.Log.Debugw("signing", "t", msg.TransactionId)
	sig, err := sigfuncs.BLSSign(sg.signer.SignKey, consensus.GetSignable(state), len(ng.Signers), int(ng.IndexOfSigner(sg.signer)))

	if err != nil {
		panic(fmt.Sprintf("error signing: %v", err))
	}

	state.Signature = sig

	context.Respond(&messages.SignatureWrapper{
		Internal:         true,
		ConflictSetID:    msg.ConflictSetID,
		Signers:          messages.SignerMap{sg.signer.ID: sg.signer},
		Metadata:         messages.MetadataMap{"seen": time.Now()},
		RewardsCommittee: committee,
		State:            state,
	})
}
