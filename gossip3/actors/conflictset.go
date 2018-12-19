package actors

import (
	"fmt"
	"time"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/Workiva/go-datastructures/bitarray"
	"github.com/quorumcontrol/tupelo/bls"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/types"
)

type signatureMap map[string]*messages.SignatureWrapper
type signaturesByTransaction map[string]signatureMap
type transactionMap map[string]*messages.TransactionWrapper

type checkStateMsg struct {
	atUpdate uint64
}

type ConflictSet struct {
	middleware.LogAwareHolder

	ID                 string
	notaryGroup        *types.NotaryGroup
	signatureGenerator *actor.PID
	signatureChecker   *actor.PID
	signatureSender    *actor.PID
	signer             *types.Signer

	done         bool
	signatures   signaturesByTransaction
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

func NewConflictSetProps(cfg *ConflictSetConfig) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &ConflictSet{
			ID:                 cfg.ID,
			notaryGroup:        cfg.NotaryGroup,
			signer:             cfg.Signer,
			signatureGenerator: cfg.SignatureGenerator,
			signatureChecker:   cfg.SignatureChecker,
			signatureSender:    cfg.SignatureSender,
			signatures:         make(signaturesByTransaction),
			transactions:       make(transactionMap),
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
		cs.handleNewSignature(context, wrapper)
	case *messages.SignatureWrapper:
		cs.handleNewSignature(context, msg)
	case *messages.CurrentState:
		cs.handleCurrentState(context, msg)
	case *messages.CurrentStateWrapper:
		cs.handleCurrentStateWrapper(context, msg)
	case *checkStateMsg:
		cs.checkState(context, msg)
	}
}

func (cs *ConflictSet) DoneReceive(context actor.Context) {
	// do nothing when in the done state
}

func (cs *ConflictSet) handleCurrentState(context actor.Context, msg *messages.CurrentState) {
	// wrap this up and then call the function directly (don't use a message)
}

func (cs *ConflictSet) handleNewTransaction(context actor.Context, msg *messages.TransactionWrapper) {
	cs.Log.Debugw("new transaction", "trans", msg.TransactionID)
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
	cs.Log.Debugw("handle new signature")
	if msg.Internal {
		cs.signatureSender.Tell(msg)
	}
	if len(msg.Signers) > 1 {
		panic(fmt.Sprintf("currently we don't handle multi signer signatures here"))
	}
	existingMap, ok := cs.signatures[string(msg.Signature.TransactionID)]
	if !ok {
		existingMap = make(signatureMap)
		for id := range msg.Signers {
			existingMap[id] = msg
		}
	} else {
		for id := range msg.Signers {
			_, ok := existingMap[id]
			if ok {
				// we already have this sig
				return
			}
			existingMap[id] = msg
		}
	}
	cs.signatures[string(msg.Signature.TransactionID)] = existingMap
	cs.updates++
	context.Self().Tell(&checkStateMsg{atUpdate: cs.updates})
}

func (cs *ConflictSet) checkState(context actor.Context, msg *checkStateMsg) {
	cs.Log.Debugw("check state")
	if cs.updates < msg.atUpdate {
		cs.Log.Debugw("old update")
		// we know there will be another check state message with a higher update
		return
	}
	if trans := cs.possiblyDone(); trans != nil {
		// we have a possibly done transaction, lets make a current state
		cs.createCurrentStateFromTrans(context, trans)

	}
}

func (cs *ConflictSet) createCurrentStateFromTrans(context actor.Context, trans *messages.TransactionWrapper) error {
	cs.Log.Debugw("createCurrentStateFromTrans")
	sigs := cs.signatures[string(trans.TransactionID)]
	var sigBytes [][]byte
	signersArray := bitarray.NewSparseBitArray()
	for _, sig := range sigs {
		other, err := bitarray.Unmarshal(sig.Signature.Signers)
		if err != nil {
			return fmt.Errorf("error unmarshaling: %v", err)
		}
		signersArray = signersArray.And(other)
		sigBytes = append(sigBytes, sig.Signature.Signature)
	}
	summed, err := bls.SumSignatures(sigBytes)
	if err != nil {
		return fmt.Errorf("error summing signatures: %v", err)
	}

	marshaled, err := bitarray.Marshal(signersArray)
	if err != nil {
		return fmt.Errorf("error marshaling bitarray: %v", err)
	}

	currState := &messages.CurrentStateWrapper{
		Internal: true,
		CurrentState: &messages.CurrentState{
			Signature: &messages.Signature{
				TransactionID: trans.TransactionID,
				ObjectID:      trans.Transaction.ObjectID,
				PreviousTip:   trans.Transaction.PreviousTip,
				NewTip:        trans.Transaction.NewTip,
				Signers:       marshaled,
				Signature:     summed,
			},
		},
		Metadata: messages.MetadataMap{"seen": time.Now()},
	}

	// don't use message passing, because we can skip a lot of processing if we're done right here
	return cs.handleCurrentStateWrapper(context, currState)
}

func (cs *ConflictSet) handleCurrentStateWrapper(context actor.Context, currWrapper *messages.CurrentStateWrapper) error {
	sig := currWrapper.CurrentState.Signature
	signerArray, err := bitarray.Unmarshal(sig.Signers)
	if err != nil {
		return fmt.Errorf("error unmarshaling: %v", err)
	}
	var verKeys [][]byte

	signers := cs.notaryGroup.AllSigners()
	for i, signer := range signers {
		isSet, err := signerArray.GetBit(uint64(i))
		if err != nil {
			return fmt.Errorf("error getting bit: %v", err)
		}
		if isSet {
			verKeys = append(verKeys, signer.VerKey.Bytes())
		}
	}

	resp, err := cs.signatureChecker.RequestFuture(&messages.SignatureVerification{
		Message:   sig.GetSignable(),
		Signature: sig.Signature,
		VerKeys:   verKeys,
	}, 10*time.Second).Result()

	if err != nil {
		return fmt.Errorf("error waiting for signature validation: %v", err)
	}

	if resp.(*messages.SignatureVerification).Verified {
		currWrapper.Metadata["verifiedAt"] = time.Now()
		currWrapper.Verified = true
		cs.done = true
		cs.Log.Debugw("done")
		context.SetBehavior(cs.DoneReceive)
		if parent := context.Parent(); parent != nil {
			parent.Tell(currWrapper)
		}
	}
	return nil
}

// returns true if one of the transactions has enough signatures
func (cs *ConflictSet) possiblyDone() *messages.TransactionWrapper {
	count := cs.notaryGroup.QuorumCount()
	for tID, sigList := range cs.signatures {
		cs.Log.Debugw("check count", "t", tID, "len", len(sigList), "quorumAt", count)
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
