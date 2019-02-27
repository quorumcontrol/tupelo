package actors

import (
	"fmt"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/AsynkronIT/protoactor-go/router"
	"github.com/Workiva/go-datastructures/bitarray"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/messages"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/middleware"
	"github.com/quorumcontrol/tupelo-go-client/gossip3/types"
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

	committee, err := ng.RewardsCommittee([]byte(msg.Transaction.NewTip), sg.signer)
	if err != nil {
		panic(fmt.Sprintf("error getting committee: %v", err))
	}

	signature := &messages.Signature{
		TransactionID: msg.TransactionID,
		ObjectID:      msg.Transaction.ObjectID,
		PreviousTip:   msg.Transaction.PreviousTip,
		NewTip:        msg.Transaction.NewTip,
		Signers:       marshaled,
	}

	sg.Log.Debugw("signing", "t", msg.Key)
	sig, err := sg.signer.SignKey.Sign(signature.GetSignable())
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
