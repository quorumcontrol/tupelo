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
	case *messages.TransactionWrapper:
		sg.handleNewTransaction(context, msg)
	}
}

func (sg *SignatureGenerator) handleNewTransaction(context actor.Context, msg *messages.TransactionWrapper) {
	ng := sg.notaryGroup
	signers := make([]bool, len(ng.Signers))
	signers[ng.IndexOfSigner(sg.signer)] = true
	sig, err := sg.signer.SignKey.Sign(append(msg.Transaction.ObjectID, append(msg.Transaction.PreviousTip, msg.Transaction.NewTip...)...))
	if err != nil {
		panic(fmt.Sprintf("error signing: %v", err))
	}
	sg.Log.Debugw("signing", "t", msg.Key)

	signature := &messages.Signature{
		TransactionID: msg.Key,
		ObjectID:      msg.Transaction.ObjectID,
		Tip:           msg.Transaction.NewTip,
		Signers:       signers,
		Signature:     sig,
		ConflictSetID: msg.ConflictSetID,
		Internal:      true,
	}
	context.Respond(signature)
}
