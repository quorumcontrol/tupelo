package actors

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/types"
)

type SignatureGenerator struct {
	middleware.LogAwareHolder

	notaryGroup *types.NotaryGroup
	signer      *types.Signer
}

func NewSignatureGeneratorProps(self *types.Signer, ng *types.NotaryGroup) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &SignatureGenerator{
			signer:      self,
			notaryGroup: ng,
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (sg *SignatureGenerator) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.NewValidatedTransaction:
		sg.handleNewTransaction(context, msg)
	}
}

func (sg *SignatureGenerator) handleNewTransaction(context actor.Context, msg *messages.NewValidatedTransaction) {
	ng := sg.notaryGroup
	signers := make([]bool, len(ng.Signers))
	signers[ng.IndexOfSigner(sg.signer)] = true
	sig, err := sg.signer.SignKey.Sign(append(msg.ObjectID, append(msg.OldTip, msg.NewTip...)...))
	if err != nil {
		panic(fmt.Sprintf("error signing: %v", err))
	}
	sg.Log.Infow("signing")

	signature := &messages.Signature{
		TransactionID: msg.TransactionID,
		ObjectID:      msg.ObjectID,
		Tip:           msg.NewTip,
		Signers:       signers,
		Signature:     sig,
		ConflictSetID: string(append(msg.ObjectID, msg.OldTip...)),
		Internal:      true,
	}
	context.Respond(signature)
}
