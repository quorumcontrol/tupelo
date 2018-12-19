package actors

import (
	"fmt"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/Workiva/go-datastructures/bitarray"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/types"
)

type signatureMap map[string]messages.SignerMap
type transactionMap map[string]*messages.TransactionWrapper

type checkStateMsg struct {
	atUpdate uint64
}

type ConflictSet struct {
	ID                 string
	notaryGroup        *types.NotaryGroup
	signatureGenerator *actor.PID
	signatureChecker   *actor.PID
	signatureSender    *actor.PID
	signer             *types.Signer

	done         bool
	signatures   signatureMap
	didSign      bool
	transactions transactionMap
	view         uint64
	updates      uint64
}

type ConflictSetConfig struct {
	ID                 string
	NotaryGroup        *types.NotaryGroup
	Signer             *types.Signer
	SignatureGenerator *actor.PID
	SignatureChecker   *actor.PID
	SignatureSender    *actor.PID
}

func NewConflictSetProps(id string) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &ConflictSet{
			ID:           id,
			signatures:   make(signatureMap),
			transactions: make(transactionMap),
		}
	}).WithMiddleware(
		middleware.LoggingMiddleware,
		plugin.Use(&middleware.LogPlugin{}),
	)
}

func (cs *ConflictSet) Receive(context actor.Context) {
	switch msg := context.Message().(type) {
	case *messages.TransactionWrapper:
		cs.handleNewTransaction(context, msg)
	// this will be an external signature
	case *messages.Signature:
		wrapper, err := sigToWrapper(msg, cs.notaryGroup, cs.signer, false)
		if err != nil {
			panic(fmt.Sprintf("error wrapping sig: %v", err))
		}
		// note the forwarding style here so that the signature wrapper handler
		// can still respond to this sender if necessary
		context.Self().Request(wrapper, context.Sender())
	// the wrapper is returned by the signature genator
	case *messages.SignatureWrapper:
		cs.handleNewSignature(context, msg)
	case *checkStateMsg:
		cs.checkState(context, msg)
	}
}

func (cs *ConflictSet) handleNewTransaction(context actor.Context, msg *messages.TransactionWrapper) {
	cs.handleNewTransaction(context, msg)
	if !msg.Accepted {
		panic(fmt.Sprintf("we should only handle accepted transactions at this level"))
	}
	cs.transactions[string(msg.TransactionID)] = msg
	// do this as a message to make sure we're doing it after all the updates have come in
	if !cs.didSign {
		context.Request(cs.signatureGenerator, msg)
		cs.didSign = true
	}
	cs.updates++
	context.Self().Tell(&checkStateMsg{atUpdate: cs.updates})
}

func (cs *ConflictSet) handleNewSignature(context actor.Context, msg *messages.SignatureWrapper) {
	if msg.Internal {
		cs.signatureSender.Tell(msg)
	}
	if len(msg.Signers) > 1 {
		panic(fmt.Sprintf("currently we don't handle multi signer signatures here"))
	}
	existingMap, ok := cs.signatures[string(msg.Signature.TransactionID)]
	if !ok {
		existingMap = msg.Signers
	} else {
		for id, signer := range msg.Signers {
			_, ok := existingMap[id]
			if ok {
				// we already have this sig
				return
			}
			existingMap[id] = signer
		}
	}
	cs.updates++
	context.Self().Tell(&checkStateMsg{atUpdate: cs.updates})
}

func (cs *ConflictSet) checkState(context actor.Context, msg *checkStateMsg) {
	if cs.updates < msg.atUpdate {
		// we know there will be another check state message with a higher update
		return
	}
	if trans := cs.possiblyDone(); trans != nil {
		// queue up a sig checker
	}

}

// returns true if one of the transactions has enough signatures
func (cs *ConflictSet) possiblyDone() *messages.TransactionWrapper {
	count := cs.notaryGroup.QuorumCount()
	for tID, sigList := range cs.signatures {
		if uint64(len(sigList)) >= count {
			return cs.transactions[tID]
		}
	}
	return nil
}

// TODO: detect deadlocks
func (cs *ConflictSet) deadlocked() bool {
	return false
}

func sigToWrapper(sig *messages.Signature, ng *types.NotaryGroup, self *types.Signer, isInternal bool) (*messages.SignatureWrapper, error) {
	signerMap := make(messages.SignerMap)
	signerBitMap, err := bitarray.Unmarshal(sig.Signers)
	if err != nil {
		return nil, fmt.Errorf("error unmarshaling bit array: %v", err)
	}
	allSigners := ng.AllSigners()
	for i, signer := range allSigners {
		isSet, err := signerBitMap.GetBit(uint64(i))
		if err != nil {
			return nil, fmt.Errorf("error getting bit: %v", err)
		}
		if isSet {
			signerMap[signer.ID] = signer
		}
	}

	conflictSetID := messages.ConflictSetID(sig.ObjectID, sig.PreviousTip)

	committee, err := ng.RewardsCommittee([]byte(conflictSetID), self)
	if err != nil {
		return nil, fmt.Errorf("error getting committee: %v", err)
	}

	return &messages.SignatureWrapper{
		Internal:         isInternal,
		ConflictSetID:    conflictSetID,
		RewardsCommittee: committee,
		Signers:          signerMap,
		Signature:        sig,
		Metadata:         messages.MetadataMap{"seen": time.Now()},
	}, nil
}
