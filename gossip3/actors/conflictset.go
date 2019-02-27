package actors

import (
	"fmt"
	"time"

	"github.com/quorumcontrol/storage"

	"github.com/AsynkronIT/protoactor-go/actor"
	"github.com/AsynkronIT/protoactor-go/plugin"
	"github.com/Workiva/go-datastructures/bitarray"
	"github.com/quorumcontrol/tupelo/bls"
	"github.com/quorumcontrol/tupelo/gossip3/messages"
	"github.com/quorumcontrol/tupelo/gossip3/middleware"
	"github.com/quorumcontrol/tupelo/gossip3/types"
)

type signaturesBySigner map[string]*messages.SignatureWrapper
type signaturesByTransaction map[string]signaturesBySigner
type transactionMap map[string]*messages.TransactionWrapper

type checkStateMsg struct {
	atUpdate uint64
}

type ConflictSet struct {
	middleware.LogAwareHolder

	ID                 string
	currentStateStore  storage.Reader
	notaryGroup        *types.NotaryGroup
	signatureGenerator *actor.PID
	signatureChecker   *actor.PID
	signatureSender    *actor.PID
	signer             *types.Signer

	done         bool
	signatures   signaturesByTransaction
	signerSigs   signaturesBySigner
	didSign      bool
	transactions transactionMap
	view         uint64
	updates      uint64
	active       bool
}

type ConflictSetConfig struct {
	ID                 string
	NotaryGroup        *types.NotaryGroup
	Signer             *types.Signer
	SignatureGenerator *actor.PID
	SignatureChecker   *actor.PID
	SignatureSender    *actor.PID
	CurrentStateStore  storage.Reader
	Active             bool
}

func NewConflictSetProps(cfg *ConflictSetConfig) *actor.Props {
	return actor.FromProducer(func() actor.Actor {
		return &ConflictSet{
			ID:                 cfg.ID,
			currentStateStore:  cfg.CurrentStateStore,
			notaryGroup:        cfg.NotaryGroup,
			signer:             cfg.Signer,
			signatureGenerator: cfg.SignatureGenerator,
			signatureChecker:   cfg.SignatureChecker,
			signatureSender:    cfg.SignatureSender,
			active:             cfg.Active,
			signatures:         make(signaturesByTransaction),
			signerSigs:         make(signaturesBySigner),
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
		cs.Log.Errorw("something called this")
	case *messages.CurrentStateWrapper:
		cs.handleCurrentStateWrapper(context, msg)
	case *messages.Store:
		cs.handleStore(context, msg)
	case *commitNotification:
		if cs.active {
			cs.handleStore(context, msg.store)
		} else {
			snoozedTransactions := len(cs.transactions)
			if snoozedTransactions > 0 {
				panic(fmt.Errorf("received commit notification with %d snoozed transactions; this should not happen", snoozedTransactions))
			}
		}
	case *checkStateMsg:
		cs.checkState(context, msg)
	case *messages.ProcessSnoozedTransactions:
		if parent := context.Parent(); parent != nil {
			for _, transaction := range cs.transactions {
				context.Request(parent, &messages.ValidateTransaction{
					Key:   transaction.Key,
					Value: transaction.Value,
				})
			}
		}
	}
}

func (cs *ConflictSet) handleStore(context actor.Context, msg *messages.Store) {
	cs.Log.Debugw("handleStore")
	var currState messages.CurrentState
	_, err := currState.UnmarshalMsg(msg.Value)
	if err != nil {
		panic(fmt.Errorf("error unmarshaling: %v", err))
	}
	wrapper := &messages.CurrentStateWrapper{
		CurrentState: &currState,
		Internal:     false,
		Key:          msg.Key,
		Value:        msg.Value,
		Metadata:     messages.MetadataMap{"seen": time.Now()},
	}
	cs.handleCurrentStateWrapper(context, wrapper)
}

func (cs *ConflictSet) DoneReceive(context actor.Context) {
	cs.Log.Debugw("done cs received message")
	// do nothing when in the done state
}

func (cs *ConflictSet) handleNewTransaction(context actor.Context, msg *messages.TransactionWrapper) {
	cs.Log.Debugw("new transaction", "trans", msg.TransactionID)
	if !msg.PreFlight && !msg.Accepted {
		panic(fmt.Sprintf("we should only handle pre-flight or accepted transactions at this level"))
	}
	if !cs.active {
		cs.Log.Debugw("snoozing transaction", "t", msg.Key, "height", msg.Transaction.Height)
	}
	cs.transactions[string(msg.TransactionID)] = msg
	if cs.active {
		cs.processTransactions(context)
	}
}

func (cs *ConflictSet) processTransactions(context actor.Context) {
	if !cs.active {
		panic(fmt.Errorf("error: processTransactions called on inactive ConflictSet"))
	}

	for _, transaction := range cs.transactions {
		cs.Log.Debugw("processing transaction", "t", transaction.Key, "height", transaction.Transaction.Height)

		if !cs.didSign {
			context.Request(cs.signatureGenerator, transaction)
			cs.didSign = true
		}
		cs.updates++
	}
	// do this as a message to make sure we're doing it after all the updates have come in
	context.Self().Tell(&checkStateMsg{atUpdate: cs.updates})
}

func (cs *ConflictSet) handleNewSignature(context actor.Context, msg *messages.SignatureWrapper) {
	cs.Log.Debugw("handle new signature", "t", msg.Signature.TransactionID)
	if msg.Internal {
		cs.signatureSender.Tell(msg)
	}
	if len(msg.Signers) > 1 {
		panic(fmt.Sprintf("currently we don't handle multi signer signatures here"))
	}
	existingMap, ok := cs.signatures[string(msg.Signature.TransactionID)]
	if !ok {
		existingMap = make(signaturesBySigner)
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
	for id := range msg.Signers {
		//Note (TB): this is probably a good place to look for slashable offenses
		cs.signerSigs[id] = msg
	}

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
		return
	}

	if cs.deadlocked() {
		cs.handleDeadlockedState(context)
	}
}

func (cs *ConflictSet) handleDeadlockedState(context actor.Context) {
	cs.Log.Debugw("handle deadlocked state")

	var lowestTrans *messages.TransactionWrapper
	for transID, trans := range cs.transactions {
		if lowestTrans == nil {
			lowestTrans = trans
			continue
		}
		if transID < string(lowestTrans.TransactionID) {
			lowestTrans = trans
		}
	}
	cs.nextView(lowestTrans)

	cs.handleNewTransaction(context, lowestTrans)
}

func (cs *ConflictSet) nextView(newWinner *messages.TransactionWrapper) {
	cs.view++
	cs.didSign = false
	cs.transactions = make(transactionMap)

	// only keep signatures on the winning transaction
	transSigs := cs.signatures[string(newWinner.TransactionID)]
	cs.signatures = signaturesByTransaction{string(newWinner.TransactionID): transSigs}
	cs.signerSigs = transSigs
}

func (cs *ConflictSet) createCurrentStateFromTrans(context actor.Context, trans *messages.TransactionWrapper) error {
	cs.Log.Debugw("createCurrentStateFromTrans", "t", trans.Key)
	sigs := cs.signatures[string(trans.TransactionID)]
	var sigBytes [][]byte
	signersArray := bitarray.NewSparseBitArray()
	for _, sig := range sigs {
		other, err := bitarray.Unmarshal(sig.Signature.Signers)
		if err != nil {
			return fmt.Errorf("error unmarshaling: %v", err)
		}
		signersArray = signersArray.Or(other)
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

	currState := &messages.CurrentState{
		Signature: &messages.Signature{
			TransactionID: trans.TransactionID,
			ObjectID:      trans.Transaction.ObjectID,
			PreviousTip:   trans.Transaction.PreviousTip,
			Height:        trans.Transaction.Height,
			NewTip:        trans.Transaction.NewTip,
			Signers:       marshaled,
			Signature:     summed,
		},
	}

	marshaledState, err := currState.MarshalMsg(nil)
	if err != nil {
		return fmt.Errorf("error marshaling: %v", err)
	}

	currStateWrapper := &messages.CurrentStateWrapper{
		Internal:     true,
		CurrentState: currState,
		Key:          currState.CommittedKey(),
		Value:        marshaledState,
		Metadata:     messages.MetadataMap{"seen": time.Now()},
	}

	// don't use message passing, because we can skip a lot of processing if we're done right here
	return cs.handleCurrentStateWrapper(context, currStateWrapper)
}

func (cs *ConflictSet) handleCurrentStateWrapper(context actor.Context, currWrapper *messages.CurrentStateWrapper) error {
	cs.Log.Debugw("handleCurrentStateWrapper", "internal", currWrapper.Internal)

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

	cs.Log.Debugw("checking signature", "len", len(verKeys))
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
		currWrapper.CleanupTransactions = make([]*messages.TransactionWrapper, len(cs.transactions), len(cs.transactions))
		i := 0
		for _, t := range cs.transactions {
			currWrapper.CleanupTransactions[i] = t
			i++
		}

		cs.done = true
		cs.Log.Debugw("done")
		context.SetBehavior(cs.DoneReceive)

		if parent := context.Parent(); parent != nil {
			parent.Tell(currWrapper)
		}
	} else {
		cs.Log.Errorw("signature not verified")
	}
	return nil
}

// returns a transaction with enough signatures or nil if none yet exist
func (cs *ConflictSet) possiblyDone() *messages.TransactionWrapper {
	count := cs.notaryGroup.QuorumCount()
	for tID, sigList := range cs.signatures {
		cs.Log.Debugw("check count", "t", []byte(tID), "len", len(sigList), "quorumAt", count)
		if uint64(len(sigList)) >= count {
			return cs.transactions[tID]
		}
	}
	return nil
}

func (cs *ConflictSet) deadlocked() bool {
	unknownSigCount := len(cs.notaryGroup.Signers) - len(cs.signerSigs)
	quorumAt := cs.notaryGroup.QuorumCount()
	if len(cs.signerSigs) == 0 {
		return false
	}
	for _, sigs := range cs.signatures {
		if uint64(len(sigs)+unknownSigCount) >= quorumAt {
			return false
		}
	}

	return true
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

	conflictSetID := messages.ConflictSetID(sig.ObjectID, sig.Height)

	committee, err := ng.RewardsCommittee([]byte(sig.NewTip), self)
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
