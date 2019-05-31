package actors

import (
	"fmt"
	"time"

	"github.com/quorumcontrol/messages/build/go/signatures"
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

	signers := make([]uint32, len(ng.Signers))
	signers[ng.IndexOfSigner(sg.signer)] = 1

	committee, err := ng.RewardsCommittee([]byte(msg.Transaction.NewTip), sg.signer)
	if err != nil {
		panic(fmt.Sprintf("error getting committee: %v", err))
	}

	signature := &signatures.Signature{
		TransactionId: msg.TransactionId,
		ObjectId:      msg.Transaction.ObjectId,
		PreviousTip:   msg.Transaction.PreviousTip,
		NewTip:        msg.Transaction.NewTip,
		Signers:       signers,
		Height:        msg.Transaction.Height,
	}

	sg.Log.Debugw("signing", "t", msg.TransactionId)
	sig, err := sg.signer.SignKey.Sign(consensus.GetSignable(signature))
	if err != nil {
		panic(fmt.Sprintf("error signing: %v", err))
	}

	signature.Signature = sig

	context.Respond(&messages.SignatureWrapper{
		Internal:         true,
		ConflictSetID:    msg.ConflictSetID,
		Signers:          messages.SignerMap{sg.signer.ID: sg.signer},
		Metadata:         messages.MetadataMap{"seen": time.Now()},
		RewardsCommittee: committee,
		Signature:        signature,
	})
}
