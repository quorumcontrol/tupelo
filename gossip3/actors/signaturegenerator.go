package actors

import (
	"fmt"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/Workiva/go-datastructures/bitarray"
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
	signers := bitarray.NewSparseBitArray()
	signers.SetBit(ng.IndexOfSigner(sg.signer))
	marshaled, err := bitarray.Marshal(signers)
	if err != nil {
		panic(fmt.Sprintf("error marshaling bitarray: %v", err))
	}

	signature := &messages.Signature{
		ObjectID:    msg.Transaction.ObjectID,
		PreviousTip: msg.Transaction.PreviousTip,
		NewTip:      msg.Transaction.NewTip,
		Signers:     marshaled,
	}

	sg.Log.Debugw("signing", "t", msg.Key)
	sig, err := sg.signer.SignKey.Sign(signature.GetSignable())
	if err != nil {
		panic(fmt.Sprintf("error signing: %v", err))
	}

	signature.Signature = sig

	context.Respond(&messages.SignatureWrapper{
		Internal:      true,
		ConflictSetID: msg.ConflictSetID,
		TransactionID: msg.Key,

		Signature: signature,
	})
}
